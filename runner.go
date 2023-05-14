package bulletml

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"math"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Runner interface {
	Update() error
}

type BulletRunner interface {
	Runner
	Position() (float64, float64)
	Vanished() bool
}

type FireContext struct{}

type NewRunnerOptions struct {
	OnBulletFired         func(BulletRunner, *FireContext)
	CurrentShootPosition  func() (float64, float64)
	CurrentTargetPosition func() (float64, float64)
	DefaultBulletSpeed    float64
	Random                *rand.Rand
	Rank                  float64
}

func NewRunner(bulletML *BulletML, opts *NewRunnerOptions) (Runner, error) {
	_opts := *opts
	if _opts.DefaultBulletSpeed == 0 {
		_opts.DefaultBulletSpeed = 1.0
	}
	if _opts.Random == nil {
		_opts.Random = rand.New(rand.NewSource(time.Now().Unix()))
	}

	runner := &runner{
		bulletML: bulletML,
		opts:     &_opts,
		bullet:   &bulletModel{},
		updateBulletPosition: func(r *runner) {
			x, y := r.opts.CurrentShootPosition()
			r.bullet.x = x
			r.bullet.y = y
		},
		actionDefTable: make(map[string]*Action),
		fireDefTable:   make(map[string]*Fire),
		bulletDefTable: make(map[string]*Bullet),
	}

	for _, b := range bulletML.Bullets {
		if b.Label != "" {
			bl := b
			runner.bulletDefTable[b.Label] = &bl
		}
	}

	for _, f := range bulletML.Fires {
		if f.Label != "" {
			fr := f
			runner.fireDefTable[f.Label] = &fr
		}
	}

	for _, a := range bulletML.Actions {
		if a.Label != "" {
			ac := a
			runner.actionDefTable[a.Label] = &ac

			if strings.HasPrefix(a.Label, "top") {
				runner.createActionProcess(&ac, nil)
			}
		}
	}

	runner.updateBulletPosition(runner)

	return runner, nil
}

type bulletModel struct {
	x, y                                     float64
	speed                                    float64
	direction                                float64
	accelSpeedHorizontal, accelSpeedVertical float64
	vanished                                 bool
}

type runner struct {
	bulletML             *BulletML
	opts                 *NewRunnerOptions
	bullet               *bulletModel
	updateBulletPosition func(*runner)
	actionProcesses      []*actionProcess
	actionDefTable       map[string]*Action
	fireDefTable         map[string]*Fire
	bulletDefTable       map[string]*Bullet
}

func (r *runner) createActionProcess(action *Action, params parameters) *actionProcess {
	p := &actionProcess{
		runner:               r,
		waitUntil:            -1,
		changeSpeedUntil:     -1,
		changeDirectionUntil: -1,
		accelUntil:           -1,
		lastShoot:            &bulletModel{},
	}

	if action != nil {
		p.pushStack(action, params)
	}

	r.actionProcesses = append(r.actionProcesses, p)

	return p
}

func (r *runner) Update() error {
	newActionProcesses := make([]*actionProcess, 0, len(r.actionProcesses))

	for _, p := range r.actionProcesses {
		if err := p.update(); err != nil {
			if err != actionProcessEnd {
				return err
			}
		} else {
			newActionProcesses = append(newActionProcesses, p)
		}
	}

	r.actionProcesses = newActionProcesses

	r.updateBulletPosition(r)

	return nil
}

func (r *runner) Position() (float64, float64) {
	return r.bullet.x, r.bullet.y
}

func (r *runner) Vanished() bool {
	return r.bullet.vanished
}

type actionProcess struct {
	ticks                                       int
	stack                                       []*actionProcessFrame
	waitUntil                                   int
	changeSpeedUntil                            int
	changeSpeedDelta, changeSpeedTarget         float64
	changeDirectionUntil                        int
	changeDirectionDelta, changeDirectionTarget float64
	accelUntil                                  int
	accelHorizontalDelta, accelHorizontalTarget float64
	accelVerticalDelta, accelVerticalTarget     float64
	lastShoot                                   *bulletModel
	runner                                      *runner
}

var actionProcessEnd = errors.New("actionProcessEnd")

func (p *actionProcess) update() error {
	if p.ticks > p.waitUntil {
		for len(p.stack) > 0 {
			top := p.stack[len(p.stack)-1]
			if err := top.update(); err != nil {
				if err == actionProcessFrameEnd {
					p.stack = p.stack[:len(p.stack)-1]
				} else if err == actionProcessFrameWait {
					break
				} else {
					return err
				}
			}
		}
	}

	if p.ticks < p.changeSpeedUntil {
		p.runner.bullet.speed += p.changeSpeedDelta
	} else if p.ticks == p.changeSpeedUntil {
		p.runner.bullet.speed = p.changeSpeedTarget
	}

	if p.ticks < p.changeDirectionUntil {
		p.runner.bullet.direction += p.changeDirectionDelta
	} else if p.ticks == p.changeDirectionUntil {
		p.runner.bullet.direction = p.changeDirectionTarget
	}

	if p.ticks < p.accelUntil {
		p.runner.bullet.accelSpeedHorizontal += p.accelHorizontalDelta
		p.runner.bullet.accelSpeedVertical += p.accelVerticalDelta
	} else if p.ticks == p.accelUntil {
		p.runner.bullet.accelSpeedHorizontal = p.accelHorizontalTarget
		p.runner.bullet.accelSpeedVertical = p.accelVerticalTarget
	}

	p.ticks++

	if len(p.stack) == 0 && p.ticks >= p.changeSpeedUntil && p.ticks >= p.changeDirectionUntil {
		return actionProcessEnd
	}

	return nil
}

func (p *actionProcess) pushStack(action *Action, params parameters) {
	f := &actionProcessFrame{
		action:        action,
		actionProcess: p,
		params:        params,
	}

	p.stack = append(p.stack, f)
}

type parameters map[string]float64

type actionProcessFrame struct {
	action        *Action
	actionIndex   int
	repeatIndex   int
	params        parameters
	actionProcess *actionProcess
}

var (
	actionProcessFrameWait = errors.New("actionProcessFrameWait")
	actionProcessFrameEnd  = errors.New("actionProcessFrameEnd")
)

func (f *actionProcessFrame) update() error {
	for f.actionIndex < len(f.action.Commands) {
		switch c := f.action.Commands[f.actionIndex].(type) {
		case Repeat:
			repeat, err := evaluateExpr(c.Times.Expr, f.params, &c.Times, f.actionProcess.runner.opts)
			if err != nil {
				return err
			}

			var action *Action
			var params parameters
			if c.Action != nil {
				action = c.Action
				params = f.params
			} else if c.ActionRef != nil {
				action, params, err = lookUpActionDefTable(c.ActionRef, f.actionProcess.runner.actionDefTable, f.params, f.actionProcess.runner.opts)
				if err != nil {
					return err
				}
			} else {
				return newBulletmlError(fmt.Sprintf("No action in <%s> element", c.XMLName.Local), &c)
			}

			prms := make(parameters)
			for k, v := range params {
				prms[k] = v
			}

			if f.repeatIndex < int(repeat) {
				prms["$loop.index"] = float64(f.repeatIndex)

				f.actionProcess.pushStack(action, prms)

				f.repeatIndex++

				return nil
			} else {
				f.repeatIndex = 0
			}
		case Fire, FireRef:
			var err error
			var fire *Fire
			var params parameters
			if fr, ok := c.(Fire); ok {
				fire = &fr
				params = f.params
			} else if r, ok := c.(FireRef); ok {
				fire, params, err = lookUpFireDefTable(&r, f.actionProcess.runner.fireDefTable, f.params, f.actionProcess.runner.opts)
				if err != nil {
					return err
				}
			}
			fireParams := params

			var bullet *Bullet
			if fire.Bullet != nil {
				bullet = fire.Bullet
			} else if fire.BulletRef != nil {
				bullet, params, err = lookUpBulletDefTable(fire.BulletRef, f.actionProcess.runner.bulletDefTable, params, f.actionProcess.runner.opts)
				if err != nil {
					return err
				}
			} else {
				return newBulletmlError(fmt.Sprintf("No bullet in <%s> element", fire.XMLName.Local), fire)
			}
			bulletParams := params

			sx, sy := f.actionProcess.runner.bullet.x, f.actionProcess.runner.bullet.y
			tx, ty := f.actionProcess.runner.opts.CurrentTargetPosition()

			var dir float64
			d := fire.Direction
			if d != nil {
				dir, err = evaluateExpr(d.Expr, fireParams, d, f.actionProcess.runner.opts)
				if err != nil {
					return err
				}
			} else if d = bullet.Direction; d != nil {
				dir, err = evaluateExpr(d.Expr, bulletParams, d, f.actionProcess.runner.opts)
				if err != nil {
					return err
				}
			}

			if d != nil {
				dir = dir * math.Pi / 180

				switch d.Type {
				case DirectionTypeAim:
					dir += math.Atan2(ty-sy, tx-sx)
				case DirectionTypeAbsolute:
					dir -= math.Pi / 2
				case DirectionTypeRelative:
					dir += f.actionProcess.runner.bullet.direction
				case DirectionTypeSequence:
					dir += f.actionProcess.lastShoot.direction
				default:
					return newBulletmlError(fmt.Sprintf("Invalid type '%s' for <%s> element", d.Type, d.XMLName.Local), d)
				}
			} else {
				dir = math.Atan2(ty-sy, tx-sx)
			}

			var speed float64
			s := fire.Speed
			if s != nil {
				speed, err = evaluateExpr(s.Expr, fireParams, s, f.actionProcess.runner.opts)
				if err != nil {
					return err
				}
			} else if s = bullet.Speed; s != nil {
				speed, err = evaluateExpr(s.Expr, bulletParams, s, f.actionProcess.runner.opts)
				if err != nil {
					return err
				}
			}

			if s != nil {
				switch s.Type {
				case SpeedTypeAbsolute:
					// Do nothing
				case SpeedTypeRelative:
					speed += f.actionProcess.runner.bullet.speed
				case SpeedTypeSequence:
					speed += f.actionProcess.lastShoot.speed
				default:
					return newBulletmlError(fmt.Sprintf("Invalid type '%s' for <%s> element", s.Type, s.XMLName.Local), s)
				}
			} else {
				speed = f.actionProcess.runner.opts.DefaultBulletSpeed
			}

			bulletRunner := &runner{
				bulletML: f.actionProcess.runner.bulletML,
				opts:     f.actionProcess.runner.opts,
				bullet: &bulletModel{
					x:         sx,
					y:         sy,
					speed:     speed,
					direction: dir,
				},
				updateBulletPosition: func(r *runner) {
					if !r.bullet.vanished {
						r.bullet.x += r.bullet.speed * math.Cos(r.bullet.direction)
						r.bullet.y += r.bullet.speed * math.Sin(r.bullet.direction)
						r.bullet.x += r.bullet.accelSpeedHorizontal
						r.bullet.y += r.bullet.accelSpeedVertical
					}
				},
				actionDefTable: f.actionProcess.runner.actionDefTable,
				fireDefTable:   f.actionProcess.runner.fireDefTable,
				bulletDefTable: f.actionProcess.runner.bulletDefTable,
			}

			p := bulletRunner.createActionProcess(nil, nil)
			for i := len(bullet.ActionOrRefs) - 1; i >= 0; i-- {
				var action *Action
				var actionParams parameters
				switch a := bullet.ActionOrRefs[i].(type) {
				case Action:
					action = &a
					actionParams = params
				case ActionRef:
					action, actionParams, err = lookUpActionDefTable(&a, f.actionProcess.runner.actionDefTable, params, f.actionProcess.runner.opts)
					if err != nil {
						return err
					}
				default:
					return newBulletmlError(fmt.Sprintf("Invalid child element of <%s>: %T", bullet.XMLName.Local, a), bullet)
				}

				p.pushStack(action, actionParams)
			}

			f.actionProcess.runner.opts.OnBulletFired(bulletRunner, &FireContext{})

			lastShoot := *bulletRunner.bullet
			f.actionProcess.lastShoot = &lastShoot
		case ChangeSpeed:
			term, err := evaluateExpr(c.Term.Expr, f.params, &c.Term, f.actionProcess.runner.opts)
			if err != nil {
				return err
			}

			speed, err := evaluateExpr(c.Speed.Expr, f.params, &c.Speed, f.actionProcess.runner.opts)
			if err != nil {
				return err
			}

			switch c.Speed.Type {
			case SpeedTypeAbsolute, SpeedTypeRelative:
				if c.Speed.Type == SpeedTypeRelative {
					speed += f.actionProcess.runner.bullet.speed
				}
				f.actionProcess.changeSpeedDelta = (speed - f.actionProcess.runner.bullet.speed) / term
				f.actionProcess.changeSpeedTarget = speed
			case SpeedTypeSequence:
				f.actionProcess.changeSpeedDelta = speed
				f.actionProcess.changeSpeedTarget = speed*term + f.actionProcess.runner.bullet.speed
			default:
				return newBulletmlError(fmt.Sprintf("Invalid type '%s' for <%s> element", c.Speed.Type, c.Speed.XMLName.Local), &c.Speed)
			}

			f.actionProcess.changeSpeedUntil = f.actionProcess.ticks + int(term)
		case ChangeDirection:
			term, err := evaluateExpr(c.Term.Expr, f.params, &c.Term, f.actionProcess.runner.opts)
			if err != nil {
				return err
			}

			dir, err := evaluateExpr(c.Direction.Expr, f.params, &c.Direction, f.actionProcess.runner.opts)
			if err != nil {
				return err
			}

			dir = dir * math.Pi / 180

			switch c.Direction.Type {
			case DirectionTypeAbsolute, DirectionTypeAim, DirectionTypeRelative:
				if c.Direction.Type == DirectionTypeAbsolute {
					dir -= math.Pi / 2
				} else if c.Direction.Type == DirectionTypeAim {
					sx, sy := f.actionProcess.runner.bullet.x, f.actionProcess.runner.bullet.y
					tx, ty := f.actionProcess.runner.opts.CurrentTargetPosition()
					dir += math.Atan2(ty-sy, tx-sx)
				} else if c.Direction.Type == DirectionTypeRelative {
					dir += f.actionProcess.runner.bullet.direction
				}

				f.actionProcess.changeDirectionDelta = normalizeDir(dir-f.actionProcess.runner.bullet.direction) / term
				f.actionProcess.changeDirectionTarget = normalizeDir(dir)
			case DirectionTypeSequence:
				f.actionProcess.changeDirectionDelta = normalizeDir(dir)
				f.actionProcess.changeDirectionTarget = normalizeDir(dir*term + f.actionProcess.runner.bullet.direction)
			default:
				return newBulletmlError(fmt.Sprintf("Invalid type '%s' for <%s> element", c.Direction.Type, c.Direction.XMLName.Local), &c.Direction)
			}

			f.actionProcess.changeDirectionUntil = f.actionProcess.ticks + int(term)
		case Accel:
			term, err := evaluateExpr(c.Term.Expr, f.params, &c.Term, f.actionProcess.runner.opts)
			if err != nil {
				return err
			}

			f.actionProcess.accelUntil = f.actionProcess.ticks + int(term)

			if c.Horizontal != nil {
				horizontal, err := evaluateExpr(c.Horizontal.Expr, f.params, c.Horizontal, f.actionProcess.runner.opts)
				if err != nil {
					return err
				}

				switch c.Horizontal.Type {
				case HorizontalTypeAbsolute, HorizontalTypeRelative:
					if c.Horizontal.Type == HorizontalTypeRelative {
						horizontal += f.actionProcess.runner.bullet.accelSpeedHorizontal
					}
					f.actionProcess.accelHorizontalDelta = (horizontal - f.actionProcess.runner.bullet.accelSpeedHorizontal) / term
					f.actionProcess.accelHorizontalTarget = horizontal
				case HorizontalTypeSequence:
					f.actionProcess.accelHorizontalDelta = horizontal
					f.actionProcess.accelHorizontalTarget = f.actionProcess.runner.bullet.accelSpeedHorizontal + horizontal*term
				default:
					return newBulletmlError(fmt.Sprintf("Invalid type '%s' for <%s> element", string(c.Horizontal.Type), c.Horizontal.XMLName.Local), c.Horizontal)
				}
			} else {
				f.actionProcess.accelHorizontalDelta = 0
				f.actionProcess.accelHorizontalTarget = f.actionProcess.runner.bullet.accelSpeedHorizontal
			}

			if c.Vertical != nil {
				vertical, err := evaluateExpr(c.Vertical.Expr, f.params, c.Vertical, f.actionProcess.runner.opts)
				if err != nil {
					return err
				}

				switch c.Vertical.Type {
				case VerticalTypeAbsolute, VerticalTypeRelative:
					if c.Vertical.Type == VerticalTypeRelative {
						vertical += f.actionProcess.runner.bullet.accelSpeedVertical
					}
					f.actionProcess.accelVerticalDelta = (vertical - f.actionProcess.runner.bullet.accelSpeedVertical) / term
					f.actionProcess.accelVerticalTarget = vertical
				case VerticalTypeSequence:
					f.actionProcess.accelVerticalDelta = vertical
					f.actionProcess.accelVerticalTarget = f.actionProcess.runner.bullet.accelSpeedVertical + vertical*term
				default:
					return newBulletmlError(fmt.Sprintf("Invalid type '%s' for <%s> element", string(c.Vertical.Type), c.Vertical.XMLName.Local), c.Vertical)
				}
			} else {
				f.actionProcess.accelVerticalDelta = 0
				f.actionProcess.accelVerticalTarget = f.actionProcess.runner.bullet.accelSpeedVertical
			}
		case Wait:
			wait, err := evaluateExpr(c.Expr, f.params, &c, f.actionProcess.runner.opts)
			if err != nil {
				return err
			}

			f.actionProcess.waitUntil = f.actionProcess.ticks + int(wait)

			f.actionIndex++

			return actionProcessFrameWait
		case Vanish:
			f.actionProcess.runner.bullet.vanished = true
		case Action, ActionRef:
			var action *Action
			var params parameters
			if a, ok := c.(Action); ok {
				action = &a
				params = f.params
			} else if r, ok := c.(ActionRef); ok {
				var err error
				action, params, err = lookUpActionDefTable(&r, f.actionProcess.runner.actionDefTable, f.params, f.actionProcess.runner.opts)
				if err != nil {
					return err
				}
			}

			f.actionProcess.pushStack(action, params)

			f.actionIndex++

			return nil
		}

		f.actionIndex++
	}

	return actionProcessFrameEnd
}

func lookUpBulletDefTable(ref *BulletRef, table map[string]*Bullet, params parameters, opts *NewRunnerOptions) (*Bullet, parameters, error) {
	t, exists := table[ref.Label]
	if !exists {
		return nil, nil, newBulletmlError(fmt.Sprintf("<%s label=\"%s\"> not found", ref.XMLName.Local, ref.Label), ref)
	}

	refParams := make(parameters)
	for i, p := range ref.Params {
		v, err := evaluateExpr(p.Expr, params, &p, opts)
		if err != nil {
			return nil, nil, err
		}

		refParams[fmt.Sprintf("$%d", i+1)] = v
	}

	return t, refParams, nil
}

func lookUpActionDefTable(ref *ActionRef, table map[string]*Action, params parameters, opts *NewRunnerOptions) (*Action, parameters, error) {
	t, exists := table[ref.Label]
	if !exists {
		return nil, nil, newBulletmlError(fmt.Sprintf("<%s label=\"%s\"> not found", ref.XMLName.Local, ref.Label), ref)
	}

	refParams := make(parameters)
	for i, p := range ref.Params {
		v, err := evaluateExpr(p.Expr, params, &p, opts)
		if err != nil {
			return nil, nil, err
		}

		refParams[fmt.Sprintf("$%d", i+1)] = v
	}

	return t, refParams, nil
}

func lookUpFireDefTable(ref *FireRef, table map[string]*Fire, params parameters, opts *NewRunnerOptions) (*Fire, parameters, error) {
	t, exists := table[ref.Label]
	if !exists {
		return nil, nil, newBulletmlError(fmt.Sprintf("<%s label=\"%s\"> not found", ref.XMLName.Local, ref.Label), ref)
	}

	refParams := make(parameters)
	for i, p := range ref.Params {
		v, err := evaluateExpr(p.Expr, params, &p, opts)
		if err != nil {
			return nil, nil, err
		}

		refParams[fmt.Sprintf("$%d", i+1)] = v
	}

	return t, refParams, nil
}

var (
	variableRegexp = regexp.MustCompile(`\$(\d+|rand|rank|loop\.index)`)
	funcRegexp     = regexp.MustCompile(`(sin|cos)\([^\)]+\)`)
)

func evaluateExpr(expr string, params parameters, node node, opts *NewRunnerOptions) (float64, error) {
	expr = strings.ReplaceAll(expr, "$", "V_")
	expr = strings.ReplaceAll(expr, "V_loop.", "V_loop_")

	root, err := parser.ParseExpr(expr)
	if err != nil {
		return 0, newBulletmlError(err.Error(), node)
	}

	return evalAst(root, params, node, opts)
}

func evalAst(node ast.Expr, params parameters, bmlNode node, opts *NewRunnerOptions) (float64, error) {
	switch e := node.(type) {
	case *ast.BinaryExpr:
		x, err := evalAst(e.X, params, bmlNode, opts)
		if err != nil {
			return 0, err
		}
		y, err := evalAst(e.Y, params, bmlNode, opts)
		if err != nil {
			return 0, err
		}
		switch e.Op {
		case token.ADD:
			return x + y, nil
		case token.SUB:
			return x - y, nil
		case token.MUL:
			return x * y, nil
		case token.QUO:
			return x / y, nil
		case token.REM:
			return float64(int64(x) % int64(y)), nil
		default:
			return 0, newBulletmlError(fmt.Sprintf("Unsupported operator: %s", e.Op.String()), bmlNode)
		}
	case *ast.UnaryExpr:
		x, err := evalAst(e.X, params, bmlNode, opts)
		if err != nil {
			return 0, err
		}
		switch e.Op {
		case token.SUB:
			return -x, nil
		default:
			return 0, newBulletmlError(fmt.Sprintf("Unsupported operator: %s", e.Op.String()), bmlNode)
		}
	case *ast.BasicLit:
		switch e.Kind {
		case token.FLOAT, token.INT:
			return strconv.ParseFloat(e.Value, 64)
		default:
			return 0, newBulletmlError(fmt.Sprintf("Unsupported literal: %s", e.Value), bmlNode)
		}
	case *ast.Ident:
		name := e.Name
		name = strings.ReplaceAll(name, "V_loop_", "V_loop.")
		name = strings.ReplaceAll(name, "V_", "$")
		switch name {
		case "$rand":
			return opts.Random.Float64(), nil
		case "$rank":
			return opts.Rank, nil
		default:
			if v, exists := params[name]; exists {
				return v, nil
			} else {
				return 0, newBulletmlError(fmt.Sprintf("Invalid variable name: %s", e.Name), bmlNode)
			}
		}
	case *ast.CallExpr:
		f, ok := e.Fun.(*ast.Ident)
		if !ok {
			var buf bytes.Buffer
			if err := format.Node(&buf, token.NewFileSet(), e.Fun); err != nil {
				return 0, newBulletmlError(err.Error(), bmlNode)
			}
			return 0, newBulletmlError(fmt.Sprintf("Unsupported function: %s", string(buf.Bytes())), bmlNode)
		}

		var args []float64
		for _, arg := range e.Args {
			v, err := evalAst(arg, params, bmlNode, opts)
			if err != nil {
				return 0, err
			}
			args = append(args, v)
		}

		switch f.Name {
		case "sin":
			if len(args) < 1 {
				return 0, newBulletmlError(fmt.Sprintf("Too few arguments for sin(): %d", len(args)), bmlNode)
			}
			return math.Sin(args[0]), nil
		case "cos":
			if len(args) < 1 {
				return 0, newBulletmlError(fmt.Sprintf("Too few arguments for cos(): %d", len(args)), bmlNode)
			}
			return math.Cos(args[0]), nil
		default:
			return 0, newBulletmlError(fmt.Sprintf("Unsupported function: %s", f.Name), bmlNode)
		}
	default:
		var buf bytes.Buffer
		if err := format.Node(&buf, token.NewFileSet(), node); err != nil {
			return 0, err
		}
		return 0, newBulletmlError(fmt.Sprintf("Unsupported expression: %s", string(buf.Bytes())), bmlNode)
	}
}

func normalizeDir(dir float64) float64 {
	for dir > math.Pi {
		dir -= math.Pi * 2
	}
	for dir < -math.Pi {
		dir += math.Pi * 2
	}
	return dir
}

package bulletml

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"math"
	"math/rand"
	"strings"
	"time"
)

// Runner runs BulletML.
type Runner interface {
	// Update updates runner state. It should be called in every loop.
	Update() error

	completed() bool
}

// BulletRunner runs BulletML and updates the state of a bullet.
type BulletRunner interface {
	Runner

	// Position returns the bullet position (x, y).
	Position() (float64, float64)

	// Vanished returns whether the bullet has vanished or not.
	Vanished() bool
}

// FireContext contains context data of fire.
type FireContext struct {
	// Fire field is the <fire> element which emits this event.
	Fire *Fire

	// Bullet field is the <bullet> element fired by this event.
	Bullet *Bullet
}

// NewRunnerOptions contains options for NewRunner function.
type NewRunnerOptions struct {
	// [Required] OnBulletFired is called when a bullet is fired.
	OnBulletFired func(BulletRunner, *FireContext)

	// [Required] CurrentShootPosition tells the runner where the shooter is.
	CurrentShootPosition func() (float64, float64)

	// [Required] CurrentTargetPosition tells the runner where the player is.
	CurrentTargetPosition func() (float64, float64)

	// DefaultBulletSpeed is the default value of bullet speed. 1.0 is used if not specified.
	DefaultBulletSpeed float64

	// Random is used as a random generator in the runner.
	Random *rand.Rand

	// Rank is the value for $rank.
	Rank float64
}

// NewRunner creates a new Runner.
func NewRunner(bulletML *BulletML, opts *NewRunnerOptions) (Runner, error) {
	_opts := *opts
	if _opts.OnBulletFired == nil {
		return nil, errors.New("OnBulletFired is required")
	}
	if _opts.CurrentShootPosition == nil {
		return nil, errors.New("CurrentShootPosition is required")
	}
	if _opts.CurrentTargetPosition == nil {
		return nil, errors.New("CurrentTargetPosition is required")
	}
	if _opts.DefaultBulletSpeed == 0 {
		_opts.DefaultBulletSpeed = 1.0
	}
	if _opts.Random == nil {
		_opts.Random = rand.New(rand.NewSource(time.Now().Unix()))
	}

	if err := prepareNodeTree(bulletML); err != nil {
		return nil, err
	}

	bulletDefTable := make(map[string]*Bullet)
	for _, b := range bulletML.Bullets {
		if b.Label != "" {
			bulletDefTable[b.Label] = b
		}
	}

	fireDefTable := make(map[string]*Fire)
	for _, f := range bulletML.Fires {
		if f.Label != "" {
			fireDefTable[f.Label] = f
		}
	}

	actionDefTable := make(map[string]*Action)
	topActions := make([]*Action, 0)
	for _, a := range bulletML.Actions {
		if a.Label != "" {
			actionDefTable[a.Label] = a

			if strings.HasPrefix(a.Label, "top") {
				topActions = append(topActions, a)
			}
		}
	}

	config := &runnerConfig{
		bulletML:       bulletML,
		opts:           &_opts,
		actionDefTable: actionDefTable,
		fireDefTable:   fireDefTable,
		bulletDefTable: bulletDefTable,
		updateBulletPosition: func(r *runner) {
			x, y := r.config.opts.CurrentShootPosition()
			r.bullet.x = x
			r.bullet.y = y
		},
	}

	m := &multiRunner{}
	for _, a := range topActions {
		b := &bulletModel{
			speed: _opts.DefaultBulletSpeed,
		}
		b.x, b.y = _opts.CurrentShootPosition()
		r := createRunner(config, b)

		r.pushStack(a, nil)

		m.runners = append(m.runners, r)
	}

	return m, nil
}

type multiRunner struct {
	runners []Runner
}

func (m *multiRunner) Update() error {
	_runners := m.runners[:0]
	for _, r := range m.runners {
		if err := r.Update(); err != nil {
			return err
		}
		if !r.completed() {
			_runners = append(_runners, r)
		}
	}
	m.runners = _runners

	return nil
}

func (m *multiRunner) completed() bool {
	return len(m.runners) == 0
}

type runnerConfig struct {
	bulletML             *BulletML
	opts                 *NewRunnerOptions
	actionDefTable       map[string]*Action
	fireDefTable         map[string]*Fire
	bulletDefTable       map[string]*Bullet
	updateBulletPosition func(*runner)
}

type bulletModel struct {
	x, y                                     float64
	speed                                    float64
	direction                                float64
	accelSpeedHorizontal, accelSpeedVertical float64
	vanished                                 bool
}

type runner struct {
	config *runnerConfig

	bullet                       *bulletModel
	bulletPrev                   *bulletModel
	bulletVxCache, bulletVyCache float64

	ticks int
	stack []*actionProcess

	waitUntil int

	changeSpeedUntil                    int
	changeSpeedDelta, changeSpeedTarget float64

	changeDirectionUntil                        int
	changeDirectionDelta, changeDirectionTarget float64

	accelUntil                                  int
	accelHorizontalDelta, accelHorizontalTarget float64
	accelVerticalDelta, accelVerticalTarget     float64

	lastShoot *bulletModel

	allActionsCompleted bool
}

func createRunner(config *runnerConfig, bullet *bulletModel) *runner {
	r := &runner{
		config:               config,
		bullet:               bullet,
		waitUntil:            -1,
		changeSpeedUntil:     -1,
		changeDirectionUntil: -1,
		accelUntil:           -1,
		lastShoot:            &bulletModel{},
	}

	return r
}

func (r *runner) lookUpBulletDefTable(node node, params parameters) (*Bullet, parameters, bool, error) {
	if b, ok := node.(*Bullet); ok {
		return b, params, true, nil
	} else if b, ok := node.(*BulletRef); ok {
		return lookUpDefTable(b, r.config.bulletDefTable, params, r)
	} else {
		return nil, nil, false, newBulletmlError(fmt.Sprintf("Invalid type: %T", node), node)
	}
}

func (r *runner) lookUpActionDefTable(node node, params parameters) (*Action, parameters, bool, error) {
	if a, ok := node.(*Action); ok {
		return a, params, true, nil
	} else if a, ok := node.(*ActionRef); ok {
		return lookUpDefTable(a, r.config.actionDefTable, params, r)
	} else {
		return nil, nil, false, newBulletmlError(fmt.Sprintf("Invalid type: %T", node), node)
	}
}

func (r *runner) lookUpFireDefTable(node node, params parameters) (*Fire, parameters, bool, error) {
	if f, ok := node.(*Fire); ok {
		return f, params, true, nil
	} else if f, ok := node.(*FireRef); ok {
		return lookUpDefTable(f, r.config.fireDefTable, params, r)
	} else {
		return nil, nil, false, newBulletmlError(fmt.Sprintf("Invalid type: %T", node), node)
	}
}

func coalesce[T, U any](x *Option[T], y *Option[U]) any {
	if v, exists := x.Get(); exists {
		return v
	} else if v, exists := y.Get(); exists {
		return v
	} else {
		return nil
	}
}

func lookUpDefTable[T any, R refType](ref R, table map[string]*T, params parameters, runner *runner) (*T, parameters, bool, error) {
	t, exists := table[ref.label()]
	if !exists {
		return nil, nil, false, newBulletmlError(fmt.Sprintf("<%s label=\"%s\"> not found", ref.xmlName(), ref.label()), ref)
	}

	refParams := make(parameters)
	dc := true
	for i, p := range ref.params() {
		v, d, err := evaluateExpr(p.compiledExpr, params, p, runner)
		if err != nil {
			return nil, nil, false, err
		}

		refParams[fmt.Sprintf("$%d", i+1)] = v
		dc = dc && d
	}

	return t, refParams, dc, nil
}

func (r *runner) pushStack(action *Action, params parameters) {
	p := &actionProcess{
		action: action,
		params: params,
		runner: r,
	}

	r.stack = append(r.stack, p)
}

func (r *runner) Update() error {
	if r.ticks > r.waitUntil {
		for len(r.stack) > 0 {
			top := r.stack[len(r.stack)-1]
			if err := top.update(); err != nil {
				if err == actionProcessEnd {
					r.stack = r.stack[:len(r.stack)-1]
				} else if err == actionProcessWait {
					break
				} else {
					return err
				}
			}
		}
	}

	if r.ticks < r.changeSpeedUntil {
		r.bullet.speed += r.changeSpeedDelta
	} else if r.ticks == r.changeSpeedUntil {
		r.bullet.speed = r.changeSpeedTarget
	}

	if r.ticks < r.changeDirectionUntil {
		r.bullet.direction += r.changeDirectionDelta
	} else if r.ticks == r.changeDirectionUntil {
		r.bullet.direction = r.changeDirectionTarget
	}

	if r.ticks < r.accelUntil {
		r.bullet.accelSpeedHorizontal += r.accelHorizontalDelta
		r.bullet.accelSpeedVertical += r.accelVerticalDelta
	} else if r.ticks == r.accelUntil {
		r.bullet.accelSpeedHorizontal = r.accelHorizontalTarget
		r.bullet.accelSpeedVertical = r.accelVerticalTarget
	}

	r.config.updateBulletPosition(r)

	if !r.allActionsCompleted {
		if len(r.stack) == 0 &&
			r.ticks > r.waitUntil &&
			r.ticks > r.changeSpeedUntil &&
			r.ticks > r.changeDirectionUntil &&
			r.ticks > r.accelUntil {
			r.allActionsCompleted = true
		}
	}

	r.ticks++

	return nil
}

func (r *runner) completed() bool {
	return r.allActionsCompleted
}

func (r *runner) Position() (float64, float64) {
	return r.bullet.x, r.bullet.y
}

func (r *runner) Vanished() bool {
	return r.bullet.vanished
}

type parameters map[string]float64

type actionProcess struct {
	action                   *Action
	actionIndex              int
	repeatIndex, repeatCount int
	repeatAction             *Action
	repeatParams             parameters
	params                   parameters
	runner                   *runner
}

var (
	actionProcessEnd  = errors.New("actionProcessEnd")
	actionProcessWait = errors.New("actionProcessWait")
)

func updateBulletPosition(r *runner) {
	if !r.bullet.vanished {
		var vx, vy float64
		if r.bulletPrev == nil ||
			!r.completed() &&
				(r.bulletPrev.speed != r.bullet.speed ||
					r.bulletPrev.direction != r.bullet.direction ||
					r.bulletPrev.accelSpeedHorizontal != r.bullet.accelSpeedHorizontal ||
					r.bulletPrev.accelSpeedVertical != r.bullet.accelSpeedVertical) {
			vx = r.bullet.speed*math.Cos(r.bullet.direction) + r.bullet.accelSpeedHorizontal
			vy = r.bullet.speed*math.Sin(r.bullet.direction) + r.bullet.accelSpeedVertical
			r.bulletVxCache = vx
			r.bulletVyCache = vy
		} else {
			vx = r.bulletVxCache
			vy = r.bulletVyCache
		}

		r.bullet.x += vx
		r.bullet.y += vy

		if r.bulletPrev == nil {
			b := *r.bullet
			r.bulletPrev = &b
		} else if !r.completed() {
			*r.bulletPrev = *r.bullet
		}
	}
}

func (p *actionProcess) update() error {
	for p.actionIndex < len(p.action.Commands) {
		switch c := p.action.Commands[p.actionIndex].(type) {
		case *Repeat:
			if p.repeatIndex == 0 {
				repeat, _, err := evaluateExpr(c.Times.compiledExpr, p.params, c.Times, p.runner)
				if err != nil {
					return err
				}
				p.repeatCount = int(repeat)

				action, params, deterministic, err := p.runner.lookUpActionDefTable(coalesce(c.Action, c.ActionRef).(node), p.params)
				if err != nil {
					return err
				}

				if deterministic {
					p.repeatAction = action

					p.repeatParams = make(parameters)
					for k, v := range params {
						p.repeatParams[k] = v
					}
				}
			}

			if p.repeatAction == nil {
				action, params, _, err := p.runner.lookUpActionDefTable(coalesce(c.Action, c.ActionRef).(node), p.params)
				if err != nil {
					return err
				}

				p.repeatAction = action

				p.repeatParams = make(parameters)
				for k, v := range params {
					p.repeatParams[k] = v
				}
			}

			if p.repeatIndex < p.repeatCount {
				p.repeatParams["$loop.index"] = float64(p.repeatIndex)

				p.runner.pushStack(p.repeatAction, p.repeatParams)

				p.repeatIndex++

				return nil
			} else {
				p.repeatIndex = 0
				p.repeatCount = 0
				p.repeatAction = nil
				p.repeatParams = nil
			}
		case *Fire, *FireRef:
			fire, params, _, err := p.runner.lookUpFireDefTable(c.(node), p.params)
			if err != nil {
				return err
			}
			fireParams := params

			bullet, params, _, err := p.runner.lookUpBulletDefTable(coalesce(fire.Bullet, fire.BulletRef).(node), params)
			if err != nil {
				return err
			}
			bulletParams := params

			sx, sy := p.runner.bullet.x, p.runner.bullet.y
			tx, ty := p.runner.config.opts.CurrentTargetPosition()

			var dir float64
			d, exists := fire.Direction.Get()
			if exists {
				dir, _, err = evaluateExpr(d.compiledExpr, fireParams, d, p.runner)
				if err != nil {
					return err
				}
			} else if d, exists = bullet.Direction.Get(); exists {
				dir, _, err = evaluateExpr(d.compiledExpr, bulletParams, d, p.runner)
				if err != nil {
					return err
				}
			}

			if exists {
				dir = dir * math.Pi / 180

				switch d.Type {
				case DirectionTypeAim:
					dir += math.Atan2(ty-sy, tx-sx)
				case DirectionTypeAbsolute:
					dir -= math.Pi / 2
				case DirectionTypeRelative:
					dir += p.runner.bullet.direction
				case DirectionTypeSequence:
					dir += p.runner.lastShoot.direction
				default:
					return newBulletmlError(fmt.Sprintf("Invalid type '%s' for <%s> element", d.Type, d.XMLName.Local), d)
				}
			} else {
				dir = math.Atan2(ty-sy, tx-sx)
			}

			var speed float64
			s, exists := fire.Speed.Get()
			if exists {
				speed, _, err = evaluateExpr(s.compiledExpr, fireParams, s, p.runner)
				if err != nil {
					return err
				}
			} else if s, exists = bullet.Speed.Get(); exists {
				speed, _, err = evaluateExpr(s.compiledExpr, bulletParams, s, p.runner)
				if err != nil {
					return err
				}
			}

			if exists {
				switch s.Type {
				case SpeedTypeAbsolute:
					// Do nothing
				case SpeedTypeRelative:
					speed += p.runner.bullet.speed
				case SpeedTypeSequence:
					speed += p.runner.lastShoot.speed
				default:
					return newBulletmlError(fmt.Sprintf("Invalid type '%s' for <%s> element", s.Type, s.XMLName.Local), s)
				}
			} else {
				speed = p.runner.config.opts.DefaultBulletSpeed
			}

			config := *p.runner.config
			config.updateBulletPosition = updateBulletPosition
			bm := bulletModel{
				x:         sx,
				y:         sy,
				speed:     speed,
				direction: dir,
			}
			bulletRunner := createRunner(&config, &bm)

			for i := len(bullet.ActionOrRefs) - 1; i >= 0; i-- {
				action, actionParams, _, err := p.runner.lookUpActionDefTable(bullet.ActionOrRefs[i].(node), params)
				if err != nil {
					return err
				}

				bulletRunner.pushStack(action, actionParams)
			}

			p.runner.config.opts.OnBulletFired(bulletRunner, &FireContext{
				Fire:   fire,
				Bullet: bullet,
			})

			*p.runner.lastShoot = *bulletRunner.bullet
		case *ChangeSpeed:
			term, _, err := evaluateExpr(c.Term.compiledExpr, p.params, c.Term, p.runner)
			if err != nil {
				return err
			}

			speed, _, err := evaluateExpr(c.Speed.compiledExpr, p.params, c.Speed, p.runner)
			if err != nil {
				return err
			}

			switch c.Speed.Type {
			case SpeedTypeAbsolute, SpeedTypeRelative:
				if c.Speed.Type == SpeedTypeRelative {
					speed += p.runner.bullet.speed
				}
				p.runner.changeSpeedDelta = (speed - p.runner.bullet.speed) / term
				p.runner.changeSpeedTarget = speed
			case SpeedTypeSequence:
				p.runner.changeSpeedDelta = speed
				p.runner.changeSpeedTarget = speed*term + p.runner.bullet.speed
			default:
				return newBulletmlError(fmt.Sprintf("Invalid type '%s' for <%s> element", c.Speed.Type, c.Speed.XMLName.Local), c.Speed)
			}

			p.runner.changeSpeedUntil = p.runner.ticks + int(term)
		case *ChangeDirection:
			term, _, err := evaluateExpr(c.Term.compiledExpr, p.params, c.Term, p.runner)
			if err != nil {
				return err
			}

			dir, _, err := evaluateExpr(c.Direction.compiledExpr, p.params, c.Direction, p.runner)
			if err != nil {
				return err
			}

			dir = dir * math.Pi / 180

			switch c.Direction.Type {
			case DirectionTypeAbsolute, DirectionTypeAim, DirectionTypeRelative:
				if c.Direction.Type == DirectionTypeAbsolute {
					dir -= math.Pi / 2
				} else if c.Direction.Type == DirectionTypeAim {
					sx, sy := p.runner.bullet.x, p.runner.bullet.y
					tx, ty := p.runner.config.opts.CurrentTargetPosition()
					dir += math.Atan2(ty-sy, tx-sx)
				} else if c.Direction.Type == DirectionTypeRelative {
					dir += p.runner.bullet.direction
				}

				p.runner.changeDirectionDelta = normalizeDir(dir-p.runner.bullet.direction) / term
				p.runner.changeDirectionTarget = normalizeDir(dir)
			case DirectionTypeSequence:
				p.runner.changeDirectionDelta = normalizeDir(dir)
				p.runner.changeDirectionTarget = normalizeDir(dir*term + p.runner.bullet.direction)
			default:
				return newBulletmlError(fmt.Sprintf("Invalid type '%s' for <%s> element", c.Direction.Type, c.Direction.XMLName.Local), c.Direction)
			}

			p.runner.changeDirectionUntil = p.runner.ticks + int(term)
		case *Accel:
			term, _, err := evaluateExpr(c.Term.compiledExpr, p.params, c.Term, p.runner)
			if err != nil {
				return err
			}

			p.runner.accelUntil = p.runner.ticks + int(term)

			if h, exists := c.Horizontal.Get(); exists {
				horizontal, _, err := evaluateExpr(h.compiledExpr, p.params, h, p.runner)
				if err != nil {
					return err
				}

				switch h.Type {
				case HorizontalTypeAbsolute, HorizontalTypeRelative:
					if h.Type == HorizontalTypeRelative {
						horizontal += p.runner.bullet.accelSpeedHorizontal
					}
					p.runner.accelHorizontalDelta = (horizontal - p.runner.bullet.accelSpeedHorizontal) / term
					p.runner.accelHorizontalTarget = horizontal
				case HorizontalTypeSequence:
					p.runner.accelHorizontalDelta = horizontal
					p.runner.accelHorizontalTarget = p.runner.bullet.accelSpeedHorizontal + horizontal*term
				default:
					return newBulletmlError(fmt.Sprintf("Invalid type '%s' for <%s> element", string(h.Type), h.XMLName.Local), h)
				}
			} else {
				p.runner.accelHorizontalDelta = 0
				p.runner.accelHorizontalTarget = p.runner.bullet.accelSpeedHorizontal
			}

			if v, exists := c.Vertical.Get(); exists {
				vertical, _, err := evaluateExpr(v.compiledExpr, p.params, v, p.runner)
				if err != nil {
					return err
				}

				switch v.Type {
				case VerticalTypeAbsolute, VerticalTypeRelative:
					if v.Type == VerticalTypeRelative {
						vertical += p.runner.bullet.accelSpeedVertical
					}
					p.runner.accelVerticalDelta = (vertical - p.runner.bullet.accelSpeedVertical) / term
					p.runner.accelVerticalTarget = vertical
				case VerticalTypeSequence:
					p.runner.accelVerticalDelta = vertical
					p.runner.accelVerticalTarget = p.runner.bullet.accelSpeedVertical + vertical*term
				default:
					return newBulletmlError(fmt.Sprintf("Invalid type '%s' for <%s> element", string(v.Type), v.XMLName.Local), v)
				}
			} else {
				p.runner.accelVerticalDelta = 0
				p.runner.accelVerticalTarget = p.runner.bullet.accelSpeedVertical
			}
		case *Wait:
			wait, _, err := evaluateExpr(c.compiledExpr, p.params, c, p.runner)
			if err != nil {
				return err
			}

			p.runner.waitUntil = p.runner.ticks + int(wait)

			p.actionIndex++

			return actionProcessWait
		case *Vanish:
			p.runner.bullet.vanished = true
		case *Action, *ActionRef:
			action, params, _, err := p.runner.lookUpActionDefTable(c.(node), p.params)
			if err != nil {
				return err
			}

			p.runner.pushStack(action, params)

			p.actionIndex++

			return nil
		}

		p.actionIndex++
	}

	return actionProcessEnd
}

func evaluateExpr(expr ast.Expr, params parameters, node node, runner *runner) (value float64, deterministic bool, err error) {
	switch e := expr.(type) {
	case *numberValue:
		return e.value, true, nil
	case *ast.BinaryExpr:
		x, xDc, err := evaluateExpr(e.X, params, node, runner)
		if err != nil {
			return 0, false, err
		}
		y, yDc, err := evaluateExpr(e.Y, params, node, runner)
		if err != nil {
			return 0, false, err
		}
		switch e.Op {
		case token.ADD:
			return x + y, xDc && yDc, nil
		case token.SUB:
			return x - y, xDc && yDc, nil
		case token.MUL:
			return x * y, xDc && yDc, nil
		case token.QUO:
			return x / y, xDc && yDc, nil
		case token.REM:
			return float64(int64(x) % int64(y)), xDc && yDc, nil
		default:
			return 0, false, newBulletmlError(fmt.Sprintf("Unsupported operator: %s", e.Op.String()), node)
		}
	case *ast.UnaryExpr:
		x, dc, err := evaluateExpr(e.X, params, node, runner)
		if err != nil {
			return 0, false, err
		}
		switch e.Op {
		case token.SUB:
			return -x, dc, nil
		default:
			return 0, false, newBulletmlError(fmt.Sprintf("Unsupported operator: %s", e.Op.String()), node)
		}
	case *ast.Ident:
		switch e.Name {
		case "$rand":
			return runner.config.opts.Random.Float64(), false, nil
		case "$rank":
			return runner.config.opts.Rank, true, nil
		case "$direction":
			b := runner.bullet
			if b.accelSpeedHorizontal == 0 && b.accelSpeedVertical == 0 {
				return b.direction*180/math.Pi + 90, false, nil
			} else {
				vx := b.speed*math.Cos(b.direction) + b.accelSpeedHorizontal
				vy := b.speed*math.Sin(b.direction) + b.accelSpeedVertical
				return math.Atan2(vy, vx)*180/math.Pi + 90, false, nil
			}
		case "$speed":
			b := runner.bullet
			if b.accelSpeedHorizontal == 0 && b.accelSpeedVertical == 0 {
				return b.speed, false, nil
			} else {
				vx := b.speed*math.Cos(b.direction) + b.accelSpeedHorizontal
				vy := b.speed*math.Sin(b.direction) + b.accelSpeedVertical
				return math.Sqrt(vx*vx + vy*vy), false, nil
			}
		default:
			if v, exists := params[e.Name]; exists {
				return v, true, nil
			} else {
				return 0, false, newBulletmlError(fmt.Sprintf("Invalid variable name: %s", e.Name), node)
			}
		}
	case *ast.CallExpr:
		f, ok := e.Fun.(*ast.Ident)
		if !ok {
			var buf bytes.Buffer
			if err := format.Node(&buf, token.NewFileSet(), e.Fun); err != nil {
				return 0, false, newBulletmlError(err.Error(), node)
			}
			return 0, false, newBulletmlError(fmt.Sprintf("Unsupported function: %s", string(buf.Bytes())), node)
		}

		var args []float64
		dc := true
		for _, arg := range e.Args {
			v, d, err := evaluateExpr(arg, params, node, runner)
			if err != nil {
				return 0, false, err
			}
			args = append(args, v)
			dc = dc && d
		}

		switch f.Name {
		case "sin":
			if len(args) < 1 {
				return 0, false, newBulletmlError(fmt.Sprintf("Too few arguments for sin(): %d", len(args)), node)
			}
			arg := args[0] * math.Pi / 180
			return math.Sin(arg), dc, nil
		case "cos":
			if len(args) < 1 {
				return 0, false, newBulletmlError(fmt.Sprintf("Too few arguments for cos(): %d", len(args)), node)
			}
			arg := args[0] * math.Pi / 180
			return math.Cos(arg), dc, nil
		default:
			return 0, false, newBulletmlError(fmt.Sprintf("Unsupported function: %s", f.Name), node)
		}
	case *ast.ParenExpr:
		return evaluateExpr(e.X, params, node, runner)
	default:
		var buf bytes.Buffer
		if err := format.Node(&buf, token.NewFileSet(), e); err != nil {
			return 0, false, err
		}
		return 0, false, newBulletmlError(fmt.Sprintf("Unsupported expression: %s", string(buf.Bytes())), node)
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

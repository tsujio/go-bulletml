package bulletml

import (
	"errors"
	"fmt"
	"go/constant"
	"go/token"
	"go/types"
	"math"
	"math/rand"
	"regexp"
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

type NewRunnerOptions struct {
	OnBulletFired         func(BulletRunner)
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

	for _, c := range bulletML.Contents {
		switch cts := c.(type) {
		case Action:
			if cts.Label != "" {
				runner.actionDefTable[cts.Label] = &cts

				if strings.HasPrefix(cts.Label, "top") {
					runner.createActionProcess(&cts, nil)
				}
			}
		case Fire:
			if cts.Label != "" {
				runner.fireDefTable[cts.Label] = &cts
			}
		case Bullet:
			if cts.Label != "" {
				runner.bulletDefTable[cts.Label] = &cts
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
	for f.actionIndex < len(f.action.Contents) {
		switch c := f.action.Contents[f.actionIndex].(type) {
		case Repeat:
			repeat, err := evaluateExpr(c.Times.Expr, f.params, f.actionProcess.runner.opts)
			if err != nil {
				return err
			}

			action, params, err := lookUpDefTable[Action, ActionRef](c.ActionOrRef, f.actionProcess.runner.actionDefTable, f.params, f.actionProcess.runner.opts)
			if err != nil {
				return err
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
			fire, params, err := lookUpDefTable[Fire, FireRef](c, f.actionProcess.runner.fireDefTable, f.params, f.actionProcess.runner.opts)
			if err != nil {
				return err
			}
			fireParams := params

			bullet, params, err := lookUpDefTable[Bullet, BulletRef](fire.BulletOrRef, f.actionProcess.runner.bulletDefTable, params, f.actionProcess.runner.opts)
			if err != nil {
				return err
			}
			bulletParams := params

			sx, sy := f.actionProcess.runner.bullet.x, f.actionProcess.runner.bullet.y
			tx, ty := f.actionProcess.runner.opts.CurrentTargetPosition()

			var dir float64
			d := fire.Direction
			if d != nil {
				dir, err = evaluateExpr(d.Expr, fireParams, f.actionProcess.runner.opts)
				if err != nil {
					return err
				}
			} else if d = bullet.Direction; d != nil {
				dir, err = evaluateExpr(d.Expr, bulletParams, f.actionProcess.runner.opts)
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
					return fmt.Errorf("Invalid type '%s' for <direction> element", d.Type)
				}
			} else {
				dir = math.Atan2(ty-sy, tx-sx)
			}

			var speed float64
			s := fire.Speed
			if s != nil {
				speed, err = evaluateExpr(s.Expr, fireParams, f.actionProcess.runner.opts)
				if err != nil {
					return err
				}
			} else if s = bullet.Speed; s != nil {
				speed, err = evaluateExpr(s.Expr, bulletParams, f.actionProcess.runner.opts)
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
					return fmt.Errorf("Invalid type '%s' for <speed> element", s.Type)
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
			for i := len(bullet.Contents) - 1; i >= 0; i-- {
				action, actionParams, err := lookUpDefTable[Action, ActionRef](bullet.Contents[i], f.actionProcess.runner.actionDefTable, params, f.actionProcess.runner.opts)
				if err != nil {
					return err
				}

				p.pushStack(action, actionParams)
			}

			f.actionProcess.runner.opts.OnBulletFired(bulletRunner)

			lastShoot := *bulletRunner.bullet
			f.actionProcess.lastShoot = &lastShoot
		case ChangeSpeed:
			term, err := evaluateExpr(c.Term.Expr, f.params, f.actionProcess.runner.opts)
			if err != nil {
				return err
			}

			speed, err := evaluateExpr(c.Speed.Expr, f.params, f.actionProcess.runner.opts)
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
				return fmt.Errorf("Invalid type '%s' for <speed> element", c.Speed.Type)
			}

			f.actionProcess.changeSpeedUntil = f.actionProcess.ticks + int(term)
		case ChangeDirection:
			term, err := evaluateExpr(c.Term.Expr, f.params, f.actionProcess.runner.opts)
			if err != nil {
				return err
			}

			dir, err := evaluateExpr(c.Direction.Expr, f.params, f.actionProcess.runner.opts)
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
				return fmt.Errorf("Invalid type '%s' for <direction> element", c.Direction.Type)
			}

			f.actionProcess.changeDirectionUntil = f.actionProcess.ticks + int(term)
		case Accel:
			term, err := evaluateExpr(c.Term.Expr, f.params, f.actionProcess.runner.opts)
			if err != nil {
				return err
			}

			f.actionProcess.accelUntil = f.actionProcess.ticks + int(term)

			if c.Horizontal != nil {
				horizontal, err := evaluateExpr(c.Horizontal.Expr, f.params, f.actionProcess.runner.opts)
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
					return fmt.Errorf("Invalid type '%s' for <horizontal> element", string(c.Horizontal.Type))
				}
			} else {
				f.actionProcess.accelHorizontalDelta = 0
				f.actionProcess.accelHorizontalTarget = f.actionProcess.runner.bullet.accelSpeedHorizontal
			}

			if c.Vertical != nil {
				vertical, err := evaluateExpr(c.Vertical.Expr, f.params, f.actionProcess.runner.opts)
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
					return fmt.Errorf("Invalid type '%s' for <vertical> element", string(c.Vertical.Type))
				}
			} else {
				f.actionProcess.accelVerticalDelta = 0
				f.actionProcess.accelVerticalTarget = f.actionProcess.runner.bullet.accelSpeedVertical
			}
		case Wait:
			wait, err := evaluateExpr(c.Expr, f.params, f.actionProcess.runner.opts)
			if err != nil {
				return err
			}

			f.actionProcess.waitUntil = f.actionProcess.ticks + int(wait)

			f.actionIndex++

			return actionProcessFrameWait
		case Vanish:
			f.actionProcess.runner.bullet.vanished = true
		case Action, ActionRef:
			action, params, err := lookUpDefTable[Action, ActionRef](c, f.actionProcess.runner.actionDefTable, f.params, f.actionProcess.runner.opts)
			if err != nil {
				return err
			}

			f.actionProcess.pushStack(action, params)

			f.actionIndex++

			return nil
		}

		f.actionIndex++
	}

	return actionProcessFrameEnd
}

func lookUpDefTable[T any, R refType](typeOrRef any, table map[string]*T, params parameters, opts *NewRunnerOptions) (*T, parameters, error) {
	if t, ok := typeOrRef.(T); ok {
		return &t, params, nil
	} else if r, ok := typeOrRef.(R); ok {
		t, exists := table[r.label()]
		if !exists {
			return nil, nil, fmt.Errorf("<%s label=\"%s\"> not found", r.xmlName(), r.label())
		}

		refParams := make(parameters)
		for i, p := range r.params() {
			v, err := evaluateExpr(p.Expr, params, opts)
			if err != nil {
				return nil, nil, err
			}

			refParams[fmt.Sprintf("$%d", i+1)] = v
		}

		return t, refParams, nil
	} else {
		return nil, nil, fmt.Errorf("Invalid type: %T", typeOrRef)
	}
}

var (
	variableRegexp = regexp.MustCompile(`\$(\d+|rand|rank|loop\.index)`)
	funcRegexp     = regexp.MustCompile(`(sin|cos)\([^\)]+\)`)
)

func evaluateExpr(expr string, params parameters, opts *NewRunnerOptions) (float64, error) {
	expr = variableRegexp.ReplaceAllStringFunc(expr, func(m string) string {
		switch m {
		case "$rand":
			return fmt.Sprintf("%f", opts.Random.Float64())
		case "$rank":
			return fmt.Sprintf("%f", opts.Rank)
		default:
			if v, exists := params[m]; exists {
				return fmt.Sprintf("%f", v)
			} else {
				return ""
			}
		}
	})

	expr = funcRegexp.ReplaceAllStringFunc(expr, func(m string) string {
		idx := strings.Index(m, "(")
		argExpr := m[idx+1 : len(m)-1]
		tv, err := types.Eval(token.NewFileSet(), nil, token.NoPos, argExpr)
		if err != nil {
			return ""
		}
		arg, _ := constant.Float64Val(tv.Value)

		switch m[:idx] {
		case "sin":
			return fmt.Sprintf("%f", math.Sin(arg))
		case "cos":
			return fmt.Sprintf("%f", math.Cos(arg))
		default:
			return ""
		}
	})

	tv, err := types.Eval(token.NewFileSet(), nil, token.NoPos, expr)
	if err != nil {
		return 0, err
	}

	v, _ := constant.Float64Val(tv.Value)

	return v, nil
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

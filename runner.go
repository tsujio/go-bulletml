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
		bulletML:       bulletML,
		opts:           &_opts,
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
					p := &actionProcess{
						runner:           runner,
						changeSpeedUntil: -1,
					}
					p.pushStack(&cts, nil)
					runner.actionProcesses = append(runner.actionProcesses, p)
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

	return runner, nil
}

type bulletModel struct {
	x, y     float64
	vx, vy   float64
	vanished bool
}

type runner struct {
	bulletML        *BulletML
	opts            *NewRunnerOptions
	bullet          *bulletModel
	actionProcesses []*actionProcess
	actionDefTable  map[string]*Action
	fireDefTable    map[string]*Fire
	bulletDefTable  map[string]*Bullet
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

	if r.bullet != nil && !r.bullet.vanished {
		r.bullet.x += r.bullet.vx
		r.bullet.y += r.bullet.vy
	}

	return nil
}

func (r *runner) Position() (float64, float64) {
	return r.bullet.x, r.bullet.y
}

func (r *runner) Vanished() bool {
	return r.bullet.vanished
}

type actionProcess struct {
	ticks             int
	stack             []*actionProcessFrame
	changeSpeedUntil  int
	changeSpeedDelta  float64
	changeSpeedTarget float64
	lastShoot         *bulletModel
	runner            *runner
}

var actionProcessEnd = errors.New("actionProcessEnd")

func (p *actionProcess) update() error {
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

	if p.ticks < p.changeSpeedUntil {
		dir := math.Atan2(p.runner.bullet.vy, p.runner.bullet.vx)
		dvx := p.changeSpeedDelta * math.Cos(dir)
		dvy := p.changeSpeedDelta * math.Sin(dir)
		p.runner.bullet.vx += dvx
		p.runner.bullet.vy += dvy
	} else if p.ticks == p.changeSpeedUntil {
		dir := math.Atan2(p.runner.bullet.vy, p.runner.bullet.vx)
		p.runner.bullet.vx = p.changeSpeedTarget * math.Cos(dir)
		p.runner.bullet.vy = p.changeSpeedTarget * math.Sin(dir)
	}

	p.ticks++

	if len(p.stack) == 0 {
		return actionProcessEnd
	}

	return nil
}

func (p *actionProcess) pushStack(action *Action, params []float64) {
	f := &actionProcessFrame{
		action:        action,
		actionProcess: p,
		params:        params,
	}

	p.stack = append(p.stack, f)
}

type actionProcessFrame struct {
	action        *Action
	actionIndex   int
	repeatIndex   int
	waitUntil     *uint64
	changeDelta   float64
	params        []float64
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

			if f.repeatIndex < int(repeat) {
				f.actionProcess.pushStack(action, params)

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

			sx, sy := f.actionProcess.runner.opts.CurrentShootPosition()
			tx, ty := f.actionProcess.runner.opts.CurrentTargetPosition()

			dir, err := calculateDirection(fire.Direction, sx, sy, tx, ty, bullet.Direction, f.actionProcess.lastShoot, fireParams, bulletParams, f.actionProcess.runner.opts)
			if err != nil {
				return err
			}

			speed, err := calculateSpeed(fire.Speed, bullet.Speed, f.actionProcess.lastShoot, fireParams, bulletParams, f.actionProcess.runner.opts)
			if err != nil {
				return err
			}

			vx := speed * math.Cos(dir)
			vy := speed * math.Sin(dir)

			bulletRunner := *f.actionProcess.runner
			bulletRunner.bullet = &bulletModel{
				x:  sx,
				y:  sy,
				vx: vx,
				vy: vy,
			}
			p := &actionProcess{
				runner:           &bulletRunner,
				changeSpeedUntil: -1,
			}
			for i := len(bullet.Contents) - 1; i >= 0; i-- {
				action, actionParams, err := lookUpDefTable[Action, ActionRef](bullet.Contents[i], f.actionProcess.runner.actionDefTable, params, f.actionProcess.runner.opts)
				if err != nil {
					return err
				}

				p.pushStack(action, actionParams)
			}
			bulletRunner.actionProcesses = []*actionProcess{p}

			f.actionProcess.runner.opts.OnBulletFired(&bulletRunner)

			lastShoot := *bulletRunner.bullet
			f.actionProcess.lastShoot = &lastShoot
		case ChangeSpeed:
			term, err := evaluateExpr(c.Term.Expr, f.params, f.actionProcess.runner.opts)
			if err != nil {
				return err
			}

			current := math.Sqrt(math.Pow(float64(f.actionProcess.runner.bullet.vx), 2) + math.Pow(float64(f.actionProcess.runner.bullet.vy), 2))
			baseSpeed := &Speed{
				Type: SpeedTypeAbsolute,
				Expr: fmt.Sprintf("%f", current),
			}
			speed, err := calculateSpeed(&c.Speed, baseSpeed, nil, f.params, nil, f.actionProcess.runner.opts)
			if err != nil {
				return err
			}

			f.actionProcess.changeSpeedUntil = f.actionProcess.ticks + int(term)
			f.actionProcess.changeSpeedDelta = (speed - current) / (term + 1)
			f.actionProcess.changeSpeedTarget = speed
		case ChangeDirection:
			if f.waitUntil == nil {
				term, err := evaluateExpr(c.Term.Expr, f.params, f.actionProcess.runner.opts)
				if err != nil {
					return err
				}

				sx, sy := f.actionProcess.runner.bullet.x, f.actionProcess.runner.bullet.y
				tx, ty := f.actionProcess.runner.opts.CurrentTargetPosition()
				current := math.Atan2(float64(f.actionProcess.runner.bullet.vy), float64(f.actionProcess.runner.bullet.vx))
				baseDir := &Direction{
					Type: DirectionTypeAbsolute,
					Expr: fmt.Sprintf("%f", current*180/math.Pi+90),
				}
				dir, err := calculateDirection(&c.Direction, sx, sy, tx, ty, baseDir, nil, f.params, nil, f.actionProcess.runner.opts)
				if err != nil {
					return err
				}

				w := uint64(f.actionProcess.ticks + int(term))
				f.waitUntil = &w

				diff := dir - current
				for diff > math.Pi {
					diff -= math.Pi * 2
				}
				for diff < -math.Pi {
					diff += math.Pi * 2
				}

				f.changeDelta = diff / term
			}

			if *f.waitUntil > uint64(f.actionProcess.ticks) {
				dir := math.Atan2(float64(f.actionProcess.runner.bullet.vy), float64(f.actionProcess.runner.bullet.vx))
				dir += f.changeDelta
				speed := math.Sqrt(math.Pow(float64(f.actionProcess.runner.bullet.vx), 2) + math.Pow(float64(f.actionProcess.runner.bullet.vy), 2))
				f.actionProcess.runner.bullet.vx = speed * math.Cos(dir)
				f.actionProcess.runner.bullet.vy = speed * math.Sin(dir)
				return actionProcessFrameWait
			} else {
				f.waitUntil = nil
				f.changeDelta = 0
			}
		case Accel:
			panic("Not implemented")
		case Wait:
			if f.waitUntil == nil {
				wait, err := evaluateExpr(c.Expr, f.params, f.actionProcess.runner.opts)
				if err != nil {
					return err
				}

				w := uint64(f.actionProcess.ticks + int(wait))
				f.waitUntil = &w
			}

			if *f.waitUntil > uint64(f.actionProcess.ticks) {
				return actionProcessFrameWait
			} else {
				f.waitUntil = nil
			}
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

func lookUpDefTable[T any, R refType](typeOrRef any, table map[string]*T, params []float64, opts *NewRunnerOptions) (*T, []float64, error) {
	if t, ok := typeOrRef.(T); ok {
		return &t, params, nil
	} else if r, ok := typeOrRef.(R); ok {
		t, exists := table[r.label()]
		if !exists {
			return nil, nil, fmt.Errorf("<%s label=\"%s\"> not found", r.xmlName(), r.label())
		}

		var refParams []float64
		for _, p := range r.params() {
			v, err := evaluateExpr(p.Expr, params, opts)
			if err != nil {
				return nil, nil, err
			}
			refParams = append(refParams, v)
		}

		return t, refParams, nil
	} else {
		return nil, nil, fmt.Errorf("Invalid type: %T", typeOrRef)
	}
}

func calculateDirection(dir *Direction, sx, sy float64, tx, ty float64, baseDir *Direction, latestShoot *bulletModel, dirParams, baseDirParams []float64, opts *NewRunnerOptions) (float64, error) {
	if dir == nil {
		if baseDir != nil {
			return calculateDirection(baseDir, sx, sy, tx, ty, nil, latestShoot, baseDirParams, nil, opts)
		} else {
			return math.Atan2(float64(ty-sy), float64(tx-sx)), nil
		}
	}

	val, err := evaluateExpr(dir.Expr, dirParams, opts)
	if err != nil {
		return 0, err
	}

	switch dir.Type {
	case DirectionTypeAim:
		d := math.Atan2(float64(ty-sy), float64(tx-sx))
		d += val * math.Pi / 180
		return d, nil
	case DirectionTypeAbsolute:
		return val*math.Pi/180 - math.Pi/2, nil
	case DirectionTypeRelative:
		d, err := calculateDirection(baseDir, sx, sy, tx, ty, nil, latestShoot, baseDirParams, nil, opts)
		if err != nil {
			return 0, err
		}
		d += val * math.Pi / 180
		return d, nil
	case DirectionTypeSequence:
		if latestShoot == nil {
			return calculateDirection(baseDir, sx, sy, tx, ty, nil, latestShoot, baseDirParams, nil, opts)
		} else {
			d := math.Atan2(float64(latestShoot.vy), float64(latestShoot.vx))
			d += val * math.Pi / 180
			return d, nil
		}
	default:
		return math.Atan2(float64(ty-sy), float64(tx-sx)), nil
	}
}

func calculateSpeed(speed *Speed, baseSpeed *Speed, latestShoot *bulletModel, params, baseParams []float64, opts *NewRunnerOptions) (float64, error) {
	if speed == nil {
		if baseSpeed != nil {
			return calculateSpeed(baseSpeed, nil, latestShoot, baseParams, nil, opts)
		} else {
			return opts.DefaultBulletSpeed, nil
		}
	}

	val, err := evaluateExpr(speed.Expr, params, opts)
	if err != nil {
		return 0, err
	}

	switch speed.Type {
	case SpeedTypeAbsolute:
		return val, nil
	case SpeedTypeRelative:
		s, err := calculateSpeed(baseSpeed, nil, latestShoot, baseParams, nil, opts)
		if err != nil {
			return 0, err
		}
		s += val
		return s, nil
	case SpeedTypeSequence:
		if latestShoot == nil {
			return calculateSpeed(baseSpeed, nil, latestShoot, baseParams, nil, opts)
		} else {
			s := math.Sqrt(math.Pow(float64(latestShoot.vx), 2) + math.Pow(float64(latestShoot.vy), 2))
			if err != nil {
				return 0, err
			}
			s += val
			return s, nil
		}
	default:
		return opts.DefaultBulletSpeed, nil
	}
}

var (
	variableRegexp = regexp.MustCompile(`\$\d+`)
	randomRegexp   = regexp.MustCompile(`\$rand`)
	rankRegexp     = regexp.MustCompile(`\$rank`)
)

func evaluateExpr(expr string, params []float64, opts *NewRunnerOptions) (float64, error) {
	expr = variableRegexp.ReplaceAllStringFunc(expr, func(m string) string {
		i, err := strconv.ParseInt(m[1:], 10, 32)
		if err != nil {
			return ""
		}
		i -= 1
		if int(i) >= len(params) {
			return ""
		}
		return fmt.Sprintf("%f", params[i])
	})

	expr = randomRegexp.ReplaceAllStringFunc(expr, func(_ string) string {
		return fmt.Sprintf("%f", opts.Random.Float64())
	})

	expr = rankRegexp.ReplaceAllStringFunc(expr, func(_ string) string {
		return fmt.Sprintf("%f", opts.Rank)
	})

	tv, err := types.Eval(token.NewFileSet(), nil, token.NoPos, expr)
	if err != nil {
		return 0, err
	}

	v, _ := constant.Float64Val(tv.Value)

	return v, nil
}

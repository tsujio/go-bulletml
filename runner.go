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
	"time"
)

type Runner interface {
	Update() error
}

type Bulleter interface {
	SetX(x float64)
	SetY(y float64)
}

type NewRunnerOptions struct {
	OnBulletFired         func(*Bullet) Bulleter
	OnBulletVanished      func(Bulleter)
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
			if cts.Label == "" {
				runner.createActionProcess(&cts, nil, nil)
			} else {
				runner.actionDefTable[cts.Label] = &cts
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

type runner struct {
	bulletML        *BulletML
	opts            *NewRunnerOptions
	actionProcesses []*actionProcess
	actionDefTable  map[string]*Action
	fireDefTable    map[string]*Fire
	bulletDefTable  map[string]*Bullet
}

func (r *runner) createActionProcess(action *Action, params []float64, bullet *bulletModel) {
	p := &actionProcess{
		bullet: bullet,
		runner: r,
	}

	if action != nil {
		p.pushStack(action, params)
	}

	r.actionProcesses = append(r.actionProcesses, p)
}

func (r *runner) Update() error {
	newActionProcesses := make([]*actionProcess, 0, len(r.actionProcesses))

	n := len(r.actionProcesses)
	for i := 0; i < n; i++ {
		a := r.actionProcesses[i]
		if err := a.update(); err != nil {
			if err != actionProcessEnd {
				return err
			}
		} else {
			newActionProcesses = append(newActionProcesses, a)
		}
	}

	if len(r.actionProcesses) > n {
		newActionProcesses = append(newActionProcesses, r.actionProcesses[n:]...)
	}

	r.actionProcesses = newActionProcesses

	return nil
}

type shootInfo struct {
	vx, vy float64
}

type actionProcess struct {
	ticks       uint64
	stack       []*actionProcessFrame
	bullet      *bulletModel
	latestShoot *shootInfo
	runner      *runner
}

var actionProcessEnd = errors.New("actionProcessEnd")

func (a *actionProcess) update() error {
	a.ticks++

	for len(a.stack) > 0 {
		top := a.stack[len(a.stack)-1]
		if err := top.update(); err != nil {
			if err == actionProcessFrameEnd {
				a.stack = a.stack[:len(a.stack)-1]
			} else if err == actionProcessFrameWait {
				break
			} else {
				return err
			}
		}
	}

	if a.bullet != nil && !a.bullet.vanished {
		a.bullet.x += a.bullet.vx
		a.bullet.y += a.bullet.vy

		a.bullet.bulleter.SetX(a.bullet.x)
		a.bullet.bulleter.SetY(a.bullet.y)
	}

	if len(a.stack) == 0 && (a.bullet == nil || a.bullet.vanished) {
		return actionProcessEnd
	}

	return nil
}

func (a *actionProcess) pushStack(action *Action, params []float64) {
	f := &actionProcessFrame{
		action:        action,
		actionProcess: a,
		params:        params,
	}

	a.stack = append(a.stack, f)
}

type bulletModel struct {
	bulleter Bulleter
	x, y     float64
	vx, vy   float64
	vanished bool
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

func (a *actionProcessFrame) update() error {
	for a.actionIndex < len(a.action.Contents) {
		switch c := a.action.Contents[a.actionIndex].(type) {
		case Repeat:
			repeat, err := evaluateExpr(c.Times.Expr, a.params, a.actionProcess.runner.opts)
			if err != nil {
				return err
			}

			action, params, err := lookUpDefTable[Action, ActionRef](c.ActionOrRef, a.actionProcess.runner.actionDefTable, a.params, a.actionProcess.runner.opts)
			if err != nil {
				return err
			}

			if a.repeatIndex < int(repeat) {
				a.actionProcess.pushStack(action, params)

				a.repeatIndex++

				return nil
			} else {
				a.repeatIndex = 0
			}
		case Fire, FireRef:
			fire, params, err := lookUpDefTable[Fire, FireRef](c, a.actionProcess.runner.fireDefTable, a.params, a.actionProcess.runner.opts)
			if err != nil {
				return err
			}
			fireParams := params

			bullet, params, err := lookUpDefTable[Bullet, BulletRef](fire.BulletOrRef, a.actionProcess.runner.bulletDefTable, params, a.actionProcess.runner.opts)
			if err != nil {
				return err
			}
			bulletParams := params

			sx, sy := a.actionProcess.runner.opts.CurrentShootPosition()
			tx, ty := a.actionProcess.runner.opts.CurrentTargetPosition()

			blt := a.actionProcess.runner.opts.OnBulletFired(bullet)

			blt.SetX(sx)
			blt.SetY(sy)

			dir, err := calculateDirection(fire.Direction, sx, sy, tx, ty, bullet.Direction, a.actionProcess.latestShoot, fireParams, bulletParams, a.actionProcess.runner.opts)
			if err != nil {
				return err
			}

			speed, err := calculateSpeed(fire.Speed, bullet.Speed, a.actionProcess.latestShoot, fireParams, bulletParams, a.actionProcess.runner.opts)
			if err != nil {
				return err
			}

			vx := speed * math.Cos(dir)
			vy := speed * math.Sin(dir)

			if len(bullet.Contents) == 0 {
				a.actionProcess.runner.createActionProcess(nil, params, &bulletModel{
					bulleter: blt,
					x:        sx,
					y:        sy,
					vx:       vx,
					vy:       vy,
				})
			} else {
				for _, bc := range bullet.Contents {
					action, actionParams, err := lookUpDefTable[Action, ActionRef](bc, a.actionProcess.runner.actionDefTable, params, a.actionProcess.runner.opts)
					if err != nil {
						return err
					}

					a.actionProcess.runner.createActionProcess(action, actionParams, &bulletModel{
						bulleter: blt,
						x:        sx,
						y:        sy,
						vx:       vx,
						vy:       vy,
					})
				}
			}

			a.actionProcess.latestShoot = &shootInfo{
				vx: vx,
				vy: vy,
			}
		case ChangeSpeed:
			if a.waitUntil == nil {
				term, err := evaluateExpr(c.Term.Expr, a.params, a.actionProcess.runner.opts)
				if err != nil {
					return err
				}

				current := math.Sqrt(math.Pow(float64(a.actionProcess.bullet.vx), 2) + math.Pow(float64(a.actionProcess.bullet.vy), 2))
				baseSpeed := &Speed{
					Type: SpeedTypeAbsolute,
					Expr: fmt.Sprintf("%f", current),
				}
				speed, err := calculateSpeed(&c.Speed, baseSpeed, nil, a.params, nil, a.actionProcess.runner.opts)
				if err != nil {
					return err
				}

				w := a.actionProcess.ticks + uint64(term)
				a.waitUntil = &w

				a.changeDelta = (speed - current) / term
			}

			if *a.waitUntil > a.actionProcess.ticks {
				dir := math.Atan2(float64(a.actionProcess.bullet.vy), float64(a.actionProcess.bullet.vx))
				dvx := a.changeDelta * math.Cos(dir)
				dvy := a.changeDelta * math.Sin(dir)
				a.actionProcess.bullet.vx += dvx
				a.actionProcess.bullet.vy += dvy
				return actionProcessFrameWait
			} else {
				a.waitUntil = nil
				a.changeDelta = 0
			}
		case ChangeDirection:
			if a.waitUntil == nil {
				term, err := evaluateExpr(c.Term.Expr, a.params, a.actionProcess.runner.opts)
				if err != nil {
					return err
				}

				sx, sy := a.actionProcess.bullet.x, a.actionProcess.bullet.y
				tx, ty := a.actionProcess.runner.opts.CurrentTargetPosition()
				current := math.Atan2(float64(a.actionProcess.bullet.vy), float64(a.actionProcess.bullet.vx))
				baseDir := &Direction{
					Type: DirectionTypeAbsolute,
					Expr: fmt.Sprintf("%f", current*180/math.Pi+90),
				}
				dir, err := calculateDirection(&c.Direction, sx, sy, tx, ty, baseDir, nil, a.params, nil, a.actionProcess.runner.opts)
				if err != nil {
					return err
				}

				w := a.actionProcess.ticks + uint64(term)
				a.waitUntil = &w

				diff := dir - current
				for diff > math.Pi {
					diff -= math.Pi * 2
				}
				for diff < -math.Pi {
					diff += math.Pi * 2
				}

				a.changeDelta = diff / term
			}

			if *a.waitUntil > a.actionProcess.ticks {
				dir := math.Atan2(float64(a.actionProcess.bullet.vy), float64(a.actionProcess.bullet.vx))
				dir += a.changeDelta
				speed := math.Sqrt(math.Pow(float64(a.actionProcess.bullet.vx), 2) + math.Pow(float64(a.actionProcess.bullet.vy), 2))
				a.actionProcess.bullet.vx = speed * math.Cos(dir)
				a.actionProcess.bullet.vy = speed * math.Sin(dir)
				return actionProcessFrameWait
			} else {
				a.waitUntil = nil
				a.changeDelta = 0
			}
		case Accel:
			panic("Not implemented")
		case Wait:
			if a.waitUntil == nil {
				wait, err := evaluateExpr(c.Expr, a.params, a.actionProcess.runner.opts)
				if err != nil {
					return err
				}

				w := a.actionProcess.ticks + uint64(wait)
				a.waitUntil = &w
			}

			if *a.waitUntil > a.actionProcess.ticks {
				return actionProcessFrameWait
			} else {
				a.waitUntil = nil
			}
		case Vanish:
			a.actionProcess.runner.opts.OnBulletVanished(a.actionProcess.bullet.bulleter)
			a.actionProcess.bullet.vanished = true
		case Action, ActionRef:
			action, params, err := lookUpDefTable[Action, ActionRef](c, a.actionProcess.runner.actionDefTable, a.params, a.actionProcess.runner.opts)
			if err != nil {
				return err
			}

			a.actionProcess.pushStack(action, params)

			a.actionIndex++

			return nil
		}

		a.actionIndex++
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

func calculateDirection(dir *Direction, sx, sy float64, tx, ty float64, baseDir *Direction, latestShoot *shootInfo, dirParams, baseDirParams []float64, opts *NewRunnerOptions) (float64, error) {
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

func calculateSpeed(speed *Speed, baseSpeed *Speed, latestShoot *shootInfo, params, baseParams []float64, opts *NewRunnerOptions) (float64, error) {
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

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

	"golang.org/x/exp/constraints"
)

type Runner interface {
	Update() error
}

type Position interface {
	constraints.Float | constraints.Integer
}

type Bulleter[T Position] interface {
	SetX(x T)
	SetY(y T)
}

type NewRunnerOptions[T Position] struct {
	OnBulletFired         func(*Bullet) Bulleter[T]
	OnBulletVanished      func(Bulleter[T])
	CurrentShootPosition  func() (T, T)
	CurrentTargetPosition func() (T, T)
	DefaultBulletSpeed    float64
	Random                *rand.Rand
}

func NewRunner[T Position](bulletML *BulletML, opts *NewRunnerOptions[T]) (Runner, error) {
	_opts := *opts
	if _opts.DefaultBulletSpeed == 0 {
		_opts.DefaultBulletSpeed = 1.0
	}
	if _opts.Random == nil {
		_opts.Random = rand.New(rand.NewSource(time.Now().Unix()))
	}

	runner := &runner[T]{
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

type runner[T Position] struct {
	bulletML        *BulletML
	opts            *NewRunnerOptions[T]
	actionProcesses []*actionProcess[T]
	actionDefTable  map[string]*Action
	fireDefTable    map[string]*Fire
	bulletDefTable  map[string]*Bullet
}

func (r *runner[T]) createActionProcess(action *Action, params []float64, bullet *bulletModel[T]) {
	p := &actionProcess[T]{
		bullet: bullet,
		runner: r,
	}

	if action != nil {
		p.stack = append(p.stack, &actionProcessFrame[T]{
			action:        action,
			actionProcess: p,
			params:        params,
		})
	}

	r.actionProcesses = append(r.actionProcesses, p)
}

func (r *runner[T]) Update() error {
	newActionProcesses := make([]*actionProcess[T], 0, len(r.actionProcesses))

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

type shootInfo[T Position] struct {
	vx, vy T
}

type actionProcess[T Position] struct {
	ticks       uint64
	stack       []*actionProcessFrame[T]
	bullet      *bulletModel[T]
	latestShoot *shootInfo[T]
	runner      *runner[T]
}

var actionProcessEnd = errors.New("actionProcessEnd")

func (a *actionProcess[T]) update() error {
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

type bulletModel[T Position] struct {
	bulleter Bulleter[T]
	x, y     T
	vx, vy   T
	vanished bool
}

type actionProcessFrame[T Position] struct {
	action        *Action
	actionIndex   int
	repeatIndex   int
	waitUntil     *uint64
	changeDelta   float64
	params        []float64
	actionProcess *actionProcess[T]
}

var (
	actionProcessFrameWait = errors.New("actionProcessFrameWait")
	actionProcessFrameEnd  = errors.New("actionProcessFrameEnd")
)

func (a *actionProcessFrame[T]) update() error {
	for a.actionIndex < len(a.action.Contents) {
		switch c := a.action.Contents[a.actionIndex].(type) {
		case Repeat:
			repeat, err := evaluateExpr(c.Times.Expr, a.params, a.actionProcess.runner.opts.Random)
			if err != nil {
				return err
			}

			action, params, err := lookUpDefTable[Action, ActionRef](c.ActionOrRef, a.actionProcess.runner.actionDefTable, a.params, a.actionProcess.runner.opts.Random)
			if err != nil {
				return err
			}

			if a.repeatIndex < int(repeat) {
				f := &actionProcessFrame[T]{
					action:        action,
					actionProcess: a.actionProcess,
					params:        params,
				}
				a.actionProcess.stack = append(a.actionProcess.stack, f)

				a.repeatIndex++

				return nil
			} else {
				a.repeatIndex = 0
			}
		case Fire, FireRef:
			fire, params, err := lookUpDefTable[Fire, FireRef](c, a.actionProcess.runner.fireDefTable, a.params, a.actionProcess.runner.opts.Random)
			if err != nil {
				return err
			}
			fireParams := params

			bullet, params, err := lookUpDefTable[Bullet, BulletRef](fire.BulletOrRef, a.actionProcess.runner.bulletDefTable, params, a.actionProcess.runner.opts.Random)
			if err != nil {
				return err
			}
			bulletParams := params

			sx, sy := a.actionProcess.runner.opts.CurrentShootPosition()
			tx, ty := a.actionProcess.runner.opts.CurrentTargetPosition()

			blt := a.actionProcess.runner.opts.OnBulletFired(bullet)

			blt.SetX(sx)
			blt.SetY(sy)

			dir, err := calculateDirection(fire.Direction, sx, sy, tx, ty, bullet.Direction, a.actionProcess.latestShoot, fireParams, bulletParams, a.actionProcess.runner.opts.Random)
			if err != nil {
				return err
			}

			speed, err := calculateSpeed(fire.Speed, bullet.Speed, a.actionProcess.latestShoot, a.actionProcess.runner.opts.DefaultBulletSpeed, fireParams, bulletParams, a.actionProcess.runner.opts.Random)
			if err != nil {
				return err
			}

			vx := T(speed * math.Cos(dir))
			vy := T(speed * math.Sin(dir))

			if len(bullet.Contents) == 0 {
				a.actionProcess.runner.createActionProcess(nil, params, &bulletModel[T]{
					bulleter: blt,
					x:        sx,
					y:        sy,
					vx:       vx,
					vy:       vy,
				})
			} else {
				for _, bc := range bullet.Contents {
					action, actionParams, err := lookUpDefTable[Action, ActionRef](bc, a.actionProcess.runner.actionDefTable, params, a.actionProcess.runner.opts.Random)
					if err != nil {
						return err
					}

					a.actionProcess.runner.createActionProcess(action, actionParams, &bulletModel[T]{
						bulleter: blt,
						x:        sx,
						y:        sy,
						vx:       vx,
						vy:       vy,
					})
				}
			}

			a.actionProcess.latestShoot = &shootInfo[T]{
				vx: vx,
				vy: vy,
			}
		case ChangeSpeed:
			if a.waitUntil == nil {
				term, err := evaluateExpr(c.Term.Expr, a.params, a.actionProcess.runner.opts.Random)
				if err != nil {
					return err
				}

				current := math.Sqrt(math.Pow(float64(a.actionProcess.bullet.vx), 2) + math.Pow(float64(a.actionProcess.bullet.vy), 2))
				baseSpeed := &Speed{
					Type: SpeedTypeAbsolute,
					Expr: fmt.Sprintf("%f", current),
				}
				speed, err := calculateSpeed[T](&c.Speed, baseSpeed, nil, a.actionProcess.runner.opts.DefaultBulletSpeed, a.params, nil, a.actionProcess.runner.opts.Random)
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
				a.actionProcess.bullet.vx += T(dvx)
				a.actionProcess.bullet.vy += T(dvy)
				return actionProcessFrameWait
			} else {
				a.waitUntil = nil
				a.changeDelta = 0
			}
		case ChangeDirection:
			if a.waitUntil == nil {
				term, err := evaluateExpr(c.Term.Expr, a.params, a.actionProcess.runner.opts.Random)
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
				dir, err := calculateDirection(&c.Direction, sx, sy, tx, ty, baseDir, nil, a.params, nil, a.actionProcess.runner.opts.Random)
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
				a.actionProcess.bullet.vx = T(speed * math.Cos(dir))
				a.actionProcess.bullet.vy = T(speed * math.Sin(dir))
				return actionProcessFrameWait
			} else {
				a.waitUntil = nil
				a.changeDelta = 0
			}
		case Wait:
			if a.waitUntil == nil {
				wait, err := evaluateExpr(c.Expr, a.params, a.actionProcess.runner.opts.Random)
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
		case ActionRef:
			action, params, err := lookUpDefTable[Action, ActionRef](c, a.actionProcess.runner.actionDefTable, a.params, a.actionProcess.runner.opts.Random)
			if err != nil {
				return err
			}

			f := &actionProcessFrame[T]{
				action:        action,
				actionProcess: a.actionProcess,
				params:        params,
			}
			a.actionProcess.stack = append(a.actionProcess.stack, f)

			a.actionIndex++

			return nil
		}

		a.actionIndex++
	}

	return actionProcessFrameEnd
}

func lookUpDefTable[T any, R refType](typeOrRef any, table map[string]*T, params []float64, random *rand.Rand) (*T, []float64, error) {
	if t, ok := typeOrRef.(T); ok {
		return &t, params, nil
	} else if r, ok := typeOrRef.(R); ok {
		t, exists := table[r.label()]
		if !exists {
			return nil, nil, fmt.Errorf("<%s label=\"%s\"> not found", r.xmlName(), r.label())
		}

		var refParams []float64
		for _, p := range r.params() {
			v, err := evaluateExpr(p.Expr, params, random)
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

func calculateDirection[T Position](dir *Direction, sx, sy T, tx, ty T, baseDir *Direction, latestShoot *shootInfo[T], dirParams, baseDirParams []float64, random *rand.Rand) (float64, error) {
	if dir == nil {
		if baseDir != nil {
			return calculateDirection(baseDir, sx, sy, tx, ty, nil, latestShoot, baseDirParams, nil, random)
		} else {
			return math.Atan2(float64(ty-sy), float64(tx-sx)), nil
		}
	}

	val, err := evaluateExpr(dir.Expr, dirParams, random)
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
		d, err := calculateDirection(baseDir, sx, sy, tx, ty, nil, latestShoot, baseDirParams, nil, random)
		if err != nil {
			return 0, err
		}
		d += val * math.Pi / 180
		return d, nil
	case DirectionTypeSequence:
		if latestShoot == nil {
			return calculateDirection(baseDir, sx, sy, tx, ty, nil, latestShoot, baseDirParams, nil, random)
		} else {
			d := math.Atan2(float64(latestShoot.vy), float64(latestShoot.vx))
			d += val * math.Pi / 180
			return d, nil
		}
	default:
		return math.Atan2(float64(ty-sy), float64(tx-sx)), nil
	}
}

func calculateSpeed[T Position](speed *Speed, baseSpeed *Speed, latestShoot *shootInfo[T], defaultSpeed float64, params, baseParams []float64, random *rand.Rand) (float64, error) {
	if speed == nil {
		if baseSpeed != nil {
			return calculateSpeed(baseSpeed, nil, latestShoot, defaultSpeed, baseParams, nil, random)
		} else {
			return defaultSpeed, nil
		}
	}

	val, err := evaluateExpr(speed.Expr, params, random)
	if err != nil {
		return 0, err
	}

	switch speed.Type {
	case SpeedTypeAbsolute:
		return val, nil
	case SpeedTypeRelative:
		s, err := calculateSpeed(baseSpeed, nil, latestShoot, defaultSpeed, baseParams, nil, random)
		if err != nil {
			return 0, err
		}
		s += val
		return s, nil
	case SpeedTypeSequence:
		if latestShoot == nil {
			return calculateSpeed(baseSpeed, nil, latestShoot, defaultSpeed, baseParams, nil, random)
		} else {
			s := math.Sqrt(math.Pow(float64(latestShoot.vx), 2) + math.Pow(float64(latestShoot.vy), 2))
			if err != nil {
				return 0, err
			}
			s += val
			return s, nil
		}
	default:
		return defaultSpeed, nil
	}
}

var (
	variableRegexp = regexp.MustCompile(`\$\d+`)
	randomRegexp   = regexp.MustCompile(`\$rand`)
)

func evaluateExpr(expr string, params []float64, random *rand.Rand) (float64, error) {
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
		return fmt.Sprintf("%f", random.Float64())
	})

	tv, err := types.Eval(token.NewFileSet(), nil, token.NoPos, expr)
	if err != nil {
		return 0, err
	}

	v, _ := constant.Float64Val(tv.Value)

	return v, nil
}

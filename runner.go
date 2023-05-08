package bulletml

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"

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
}

func NewRunner[T Position](bulletML *BulletML, opts *NewRunnerOptions[T]) (Runner, error) {
	_opts := *opts
	if _opts.DefaultBulletSpeed == 0 {
		_opts.DefaultBulletSpeed = 1.0
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
				p := &actionProcess[T]{
					runner: runner,
				}
				p.stack = append(p.stack, &actionProcessFrame[T]{
					action:        &cts,
					actionProcess: p,
				})
				runner.actionProcesses = append(runner.actionProcesses, p)
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
			} else {
				return err
			}
		} else {
			break
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

var actionProcessFrameEnd = errors.New("actionProcessFrameEnd")

func (a *actionProcessFrame[T]) update() error {
	for a.actionIndex < len(a.action.Contents) {
		switch c := a.action.Contents[a.actionIndex].(type) {
		case Repeat:
			repeat, err := evaluateExpr(c.Times.Expr, a.params)
			if err != nil {
				return err
			}

			var action *Action
			switch ac := c.ActionOrRef.(type) {
			case Action:
				action = &ac
			}

			if a.repeatIndex < int(repeat) {
				f := &actionProcessFrame[T]{
					action:        action,
					actionProcess: a.actionProcess,
					params:        a.params,
				}
				a.actionProcess.stack = append(a.actionProcess.stack, f)

				a.repeatIndex++

				return nil
			} else {
				a.repeatIndex = 0
			}
		case Fire, FireRef:
			var fire *Fire
			var fireParams []float64
			if f, ok := c.(Fire); ok {
				fire = &f
				fireParams = a.params
			} else if r, ok := c.(FireRef); ok {
				f, exists := a.actionProcess.runner.fireDefTable[r.Label]
				if !exists {
					return fmt.Errorf("<fire label=\"%s\"> not found", r.Label)
				}

				for _, p := range r.Params {
					v, err := evaluateExpr(p.Expr, a.params)
					if err != nil {
						return err
					}
					fireParams = append(fireParams, v)
				}

				fire = f
			}

			var bullet *Bullet
			var bulletParams []float64
			if b, ok := fire.BulletOrRef.(Bullet); ok {
				bullet = &b
				bulletParams = fireParams
			} else if r, ok := fire.BulletOrRef.(BulletRef); ok {
				b, exists := a.actionProcess.runner.bulletDefTable[r.Label]
				if !exists {
					return fmt.Errorf("<bullet label=\"%s\"> not found", r.Label)
				}

				for _, p := range r.Params {
					v, err := evaluateExpr(p.Expr, fireParams)
					if err != nil {
						return err
					}
					bulletParams = append(bulletParams, v)
				}

				bullet = b
			}

			sx, sy := a.actionProcess.runner.opts.CurrentShootPosition()
			tx, ty := a.actionProcess.runner.opts.CurrentTargetPosition()

			blt := a.actionProcess.runner.opts.OnBulletFired(bullet)
			blt.SetX(sx)
			blt.SetY(sy)

			dir, err := calculateDirection(fire.Direction, sx, sy, tx, ty, bullet.Direction, a.actionProcess.latestShoot, fireParams, bulletParams)
			if err != nil {
				return err
			}

			speed, err := calculateSpeed(fire.Speed, bullet.Speed, a.actionProcess.latestShoot, a.actionProcess.runner.opts.DefaultBulletSpeed, fireParams, bulletParams)
			if err != nil {
				return err
			}

			vx := T(speed * math.Cos(dir))
			vy := T(speed * math.Sin(dir))

			if len(bullet.Contents) == 0 {
				p := &actionProcess[T]{
					bullet: &bulletModel[T]{
						bulleter: blt,
						x:        sx,
						y:        sy,
						vx:       vx,
						vy:       vy,
					},
					runner: a.actionProcess.runner,
				}
				a.actionProcess.runner.actionProcesses = append(a.actionProcess.runner.actionProcesses, p)
			} else {
				for _, bc := range bullet.Contents {
					var ba *Action
					switch bcts := bc.(type) {
					case Action:
						ba = &bcts
					}

					p := &actionProcess[T]{
						bullet: &bulletModel[T]{
							bulleter: blt,
							x:        sx,
							y:        sy,
							vx:       vx,
							vy:       vy,
						},
						runner: a.actionProcess.runner,
					}
					if ba != nil {
						p.stack = append(p.stack, &actionProcessFrame[T]{
							action:        ba,
							actionProcess: p,
							params:        bulletParams,
						})
					}
					a.actionProcess.runner.actionProcesses = append(a.actionProcess.runner.actionProcesses, p)
				}
			}

			a.actionProcess.latestShoot = &shootInfo[T]{
				vx: vx,
				vy: vy,
			}
		case ChangeSpeed:
			if a.waitUntil == nil {
				term, err := evaluateExpr(c.Term.Expr, a.params)
				if err != nil {
					return err
				}

				speed, err := evaluateExpr(c.Speed.Expr, a.params)
				if err != nil {
					return err
				}

				current := math.Sqrt(math.Pow(float64(a.actionProcess.bullet.vx), 2) + math.Pow(float64(a.actionProcess.bullet.vy), 2))

				switch c.Speed.Type {
				case SpeedTypeRelative:
					speed += current
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
				return nil
			} else {
				a.waitUntil = nil
				a.changeDelta = 0
			}
		case ChangeDirection:
			if a.waitUntil == nil {
				term, err := evaluateExpr(c.Term.Expr, a.params)
				if err != nil {
					return err
				}

				dir, err := evaluateExpr(c.Direction.Expr, a.params)
				if err != nil {
					return err
				}

				current := math.Atan2(float64(a.actionProcess.bullet.vy), float64(a.actionProcess.bullet.vx))

				switch c.Direction.Type {
				case DirectionTypeAim:
					tx, ty := a.actionProcess.runner.opts.CurrentTargetPosition()
					d := math.Atan2(float64(ty-a.actionProcess.bullet.y), float64(tx-a.actionProcess.bullet.x))
					dir += d
				case DirectionTypeRelative:
					dir += current
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
				return nil
			} else {
				a.waitUntil = nil
				a.changeDelta = 0
			}
		case Wait:
			if a.waitUntil == nil {
				wait, err := evaluateExpr(c.Expr, a.params)
				if err != nil {
					return err
				}

				w := a.actionProcess.ticks + uint64(wait)
				a.waitUntil = &w
				return nil
			} else if *a.waitUntil > a.actionProcess.ticks {
				return nil
			} else {
				a.waitUntil = nil
			}
		case Vanish:
			a.actionProcess.runner.opts.OnBulletVanished(a.actionProcess.bullet.bulleter)
			a.actionProcess.bullet.vanished = true
		case ActionRef:
			action, exists := a.actionProcess.runner.actionDefTable[c.Label]
			if !exists {
				return fmt.Errorf("<action label=\"%s\"> not found", c.Label)
			}

			var params []float64
			for _, p := range c.Params {
				v, err := evaluateExpr(p.Expr, a.params)
				if err != nil {
					return err
				}
				params = append(params, v)
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

func calculateDirection[T Position](dir *Direction, sx, sy T, tx, ty T, baseDir *Direction, latestShoot *shootInfo[T], dirParams, baseDirParams []float64) (float64, error) {
	if dir == nil {
		if baseDir != nil {
			return calculateDirection(baseDir, sx, sy, tx, ty, nil, latestShoot, baseDirParams, nil)
		} else {
			return math.Atan2(float64(ty-sy), float64(tx-sx)), nil
		}
	}

	val, err := evaluateExpr(dir.Expr, dirParams)
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
		d, err := calculateDirection(baseDir, sx, sy, tx, ty, nil, latestShoot, baseDirParams, nil)
		if err != nil {
			return 0, err
		}
		d += val * math.Pi / 180
		return d, nil
	case DirectionTypeSequence:
		if latestShoot == nil {
			return calculateDirection(baseDir, sx, sy, tx, ty, nil, latestShoot, baseDirParams, nil)
		} else {
			d := math.Atan2(float64(latestShoot.vy), float64(latestShoot.vx))
			d += val * math.Pi / 180
			return d, nil
		}
	default:
		return math.Atan2(float64(ty-sy), float64(tx-sx)), nil
	}
}

func calculateSpeed[T Position](speed *Speed, baseSpeed *Speed, latestShoot *shootInfo[T], defaultSpeed float64, params, baseParams []float64) (float64, error) {
	if speed == nil {
		if baseSpeed != nil {
			return calculateSpeed(baseSpeed, nil, latestShoot, defaultSpeed, baseParams, nil)
		} else {
			return defaultSpeed, nil
		}
	}

	val, err := evaluateExpr(speed.Expr, params)
	if err != nil {
		return 0, err
	}

	switch speed.Type {
	case SpeedTypeAbsolute:
		return val, nil
	case SpeedTypeRelative:
		s, err := calculateSpeed(baseSpeed, nil, latestShoot, defaultSpeed, baseParams, nil)
		if err != nil {
			return 0, err
		}
		s += val
		return s, nil
	case SpeedTypeSequence:
		if latestShoot == nil {
			return calculateSpeed(baseSpeed, nil, latestShoot, defaultSpeed, baseParams, nil)
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

var variableRegexp = regexp.MustCompile(`\$\d+`)

func evaluateExpr(expr string, params []float64) (float64, error) {
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

	return strconv.ParseFloat(expr, 64)
}

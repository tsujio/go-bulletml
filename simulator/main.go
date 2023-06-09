package main

import (
	"fmt"
	"image/color"
	"io"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"github.com/tsujio/go-bulletml"
)

const (
	screenWidth  = 480
	screenHeight = 640
)

var touchID *ebiten.TouchID

func isJustPressed() bool {
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return true
	}

	if touchIDs := inpututil.AppendJustPressedTouchIDs(nil); len(touchIDs) > 0 {
		touchID = &touchIDs[0]
		return true
	}

	return false
}

func isJustReleased() bool {
	if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
		return true
	}

	if touchID != nil && inpututil.IsTouchJustReleased(*touchID) {
		return true
	}

	return false
}

func cursorPosition() (float64, float64) {
	if touchID != nil {
		x, y := ebiten.TouchPosition(*touchID)
		return float64(x), float64(y)
	}

	x, y := ebiten.CursorPosition()
	return float64(x), float64(y)
}

type Player struct {
	x, y    float64
	dragged bool
}

func (p *Player) update(game *Game) error {
	if isJustPressed() {
		x, y := cursorPosition()
		if math.Pow(p.x-x, 2)+math.Pow(p.y-y, 2) < math.Pow(60, 2) {
			p.dragged = true
		}
	}

	if isJustReleased() {
		p.dragged = false
	}

	if p.dragged {
		p.x, p.y = cursorPosition()
	}

	if p.x < 0 {
		p.x = 0
	}
	if p.x > screenWidth {
		p.x = screenWidth
	}
	if p.y < 0 {
		p.y = 0
	}
	if p.y > screenHeight {
		p.y = screenHeight
	}

	return nil
}

func (p *Player) draw(dst *ebiten.Image) {
	vector.DrawFilledCircle(dst, float32(p.x), float32(p.y), 4, color.RGBA{0xff, 0xff, 0, 0xff}, true)
}

type Enemy struct {
	x, y    float64
	runner  bulletml.Runner
	dragged bool
}

func (e *Enemy) update(game *Game) error {
	if !game.player.dragged {
		if isJustPressed() {
			x, y := cursorPosition()
			if math.Pow(e.x-x, 2)+math.Pow(e.y-y, 2) < math.Pow(60, 2) {
				e.dragged = true
			}
		}

		if isJustReleased() {
			e.dragged = false
		}

		if e.dragged {
			e.x, e.y = cursorPosition()
		}
	}

	if err := e.runner.Update(); err != nil {
		return err
	}

	if e.x < 0 {
		e.x = 0
	}
	if e.x > screenWidth {
		e.x = screenWidth
	}
	if e.y < 0 {
		e.y = 0
	}
	if e.y > screenHeight {
		e.y = screenHeight
	}

	return nil
}

func (e *Enemy) draw(dst *ebiten.Image) {
	vector.DrawFilledCircle(dst, float32(e.x), float32(e.y), 4, color.RGBA{0xff, 0, 0, 0xff}, true)
}

type Bullet struct {
	x, y   float64
	runner bulletml.BulletRunner
}

var bulletImg = func() *ebiten.Image {
	img := ebiten.NewImage(6, 6)
	vector.DrawFilledCircle(img, 3, 3, 3, color.White, true)
	return img
}()

func (b *Bullet) update(game *Game) error {
	if err := b.runner.Update(); err != nil {
		return err
	}

	b.x, b.y = b.runner.Position()

	return nil
}

func (b *Bullet) draw(dst *ebiten.Image) {
	if b.x > -10 && b.x < screenWidth+10 && b.y > -10 && b.y < screenHeight+10 {
		opts := &ebiten.DrawImageOptions{}
		opts.GeoM.Translate(b.x-3, b.y-3)
		dst.DrawImage(bulletImg, opts)
	}
}

type sample struct {
	name string
	bml  *bulletml.BulletML
}

type Game struct {
	samples       []sample
	index         int
	player        *Player
	enemies       []*Enemy
	bullets       []*Bullet
	errorCallback func(error)
	errorOccurred bool
}

func (g *Game) appendSample(name string, source io.Reader) {
	bml, err := bulletml.Load(source)
	if err != nil {
		g.notifyError(err)
		return
	}

	s := sample{
		name: name,
		bml:  bml,
	}

	g.samples = append(g.samples, s)

	sort.Slice(g.samples, func(i, j int) bool {
		return g.samples[i].name < g.samples[j].name
	})
}

func (g *Game) Update() error {
	if len(g.samples) > 0 {
		for _, k := range inpututil.AppendJustPressedKeys(nil) {
			if k == ebiten.KeyArrowUp || k == ebiten.KeyArrowDown {
				if k == ebiten.KeyArrowUp {
					g.index = (g.index + 1) % len(g.samples)
				} else {
					g.index = (g.index + len(g.samples) - 1) % len(g.samples)
				}

				g.initializeRunner()
			}
		}
	}

	if g.errorOccurred {
		return nil
	}

	if err := g.player.update(g); err != nil {
		g.notifyError(err)
	}

	for _, e := range g.enemies {
		if err := e.update(g); err != nil {
			g.notifyError(err)
		}
	}

	for i, n := 0, len(g.bullets); i < n; i++ {
		b := g.bullets[i]
		if err := b.update(g); err != nil {
			g.notifyError(err)
		}
	}

	_bullets := g.bullets[:0]
	for _, b := range g.bullets {
		if !b.runner.Vanished() &&
			b.x > -screenWidth && b.x < screenWidth*2 && b.y > -screenHeight && b.y < screenHeight*2 {
			_bullets = append(_bullets, b)
		}
	}
	g.bullets = _bullets

	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(color.Black)

	for _, e := range g.enemies {
		e.draw(screen)
	}

	g.player.draw(screen)

	for _, b := range g.bullets {
		b.draw(screen)
	}

	ebitenutil.DebugPrint(screen, fmt.Sprintf("%.1ffps\n%d bullets", ebiten.CurrentFPS(), len(g.bullets)))

	if len(g.samples) > 0 {
		ebitenutil.DebugPrintAt(screen, g.samples[g.index].name, screenWidth-len(g.samples[g.index].name)*6, 0)
	}
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

func (g *Game) initializeRunner() {
	if len(g.samples) == 0 {
		return
	}

	enemy := &Enemy{
		x: screenWidth / 2,
		y: screenHeight * 1 / 5,
	}

	if len(g.enemies) > 0 {
		enemy.x, enemy.y = g.enemies[0].x, g.enemies[0].y
	}

	opts := &bulletml.NewRunnerOptions{
		OnBulletFired: func(bulletRunner bulletml.BulletRunner, _ *bulletml.FireContext) {
			x, y := bulletRunner.Position()
			b := &Bullet{
				x:      x,
				y:      y,
				runner: bulletRunner,
			}
			g.bullets = append(g.bullets, b)
		},
		CurrentShootPosition: func() (float64, float64) {
			return enemy.x, enemy.y
		},
		CurrentTargetPosition: func() (float64, float64) {
			return g.player.x, g.player.y
		},
	}

	runner, err := bulletml.NewRunner(g.samples[g.index].bml, opts)
	if err != nil {
		g.notifyError(err)
	}

	enemy.runner = runner

	g.enemies = []*Enemy{enemy}

	g.bullets = nil

	g.errorOccurred = false
}

func (g *Game) notifyError(err error) {
	g.errorOccurred = true

	if g.errorCallback != nil {
		g.errorCallback(err)
	} else {
		panic(err)
	}
}

var game *Game

func main() {
	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowTitle("BulletML Samples")

	game = &Game{
		player: &Player{
			x: screenWidth / 2,
			y: screenHeight * 4 / 5,
		},
	}

	for _, filename := range os.Args[1:] {
		if strings.HasSuffix(filename, ".xml") {
			f, err := os.Open(filename)
			if err != nil {
				panic(err)
			}
			game.appendSample(filename, f)
			f.Close()
		}
	}

	game.initializeRunner()

	if err := ebiten.RunGame(game); err != nil {
		panic(err)
	}
}

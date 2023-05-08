package main

import (
	"bytes"
	"embed"
	"fmt"
	"image/color"
	"sort"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"github.com/tsujio/go-bulletml"
)

const (
	screenWidth  = 640
	screenHeight = 480
)

//go:embed *.xml
var static embed.FS

var samples []struct {
	Name string
	Data *bulletml.BulletML
}

func init() {
	entries, err := static.ReadDir(".")
	if err != nil {
		panic(err)
	}

	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".xml") {
			b, err := static.ReadFile(e.Name())
			if err != nil {
				panic(err)
			}
			ml, err := bulletml.Load(bytes.NewReader(b))
			if err != nil {
				panic(err)
			}
			s := struct {
				Name string
				Data *bulletml.BulletML
			}{
				e.Name(),
				ml,
			}
			samples = append(samples, s)
		}
	}

	sort.Slice(samples, func(i, j int) bool {
		return samples[i].Name < samples[j].Name
	})
}

type Game struct {
	index            int
	runner           bulletml.Runner
	playerX, playerY float64
	enemyX, enemyY   float64
	bullets          []*bullet
}

func (g *Game) Update() error {
	if err := g.runner.Update(); err != nil {
		panic(err)
	}

	for _, k := range inpututil.AppendJustPressedKeys(nil) {
		if k == ebiten.KeyArrowUp || k == ebiten.KeyArrowDown {
			if k == ebiten.KeyArrowUp {
				g.index = (g.index + 1) % len(samples)
			} else {
				g.index = (g.index + len(samples) - 1) % len(samples)
			}

			g.initializeRunner()
		}
	}

	x, y := ebiten.CursorPosition()
	g.playerX = float64(x)
	g.playerY = float64(y)

	newBullets := make([]*bullet, 0, len(g.bullets))
	for _, b := range g.bullets {
		if !b.vanished {
			newBullets = append(newBullets, b)
		}
	}
	g.bullets = newBullets

	return nil
}

var img = func() *ebiten.Image {
	img := ebiten.NewImage(6, 6)
	img.Fill(color.Transparent)
	vector.DrawFilledCircle(img, 3, 3, 3, color.White, true)
	return img
}()

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(color.Black)

	vector.DrawFilledCircle(screen, float32(g.playerX), float32(g.playerY), 4, color.RGBA{0xff, 0xff, 0, 0xff}, true)

	vector.DrawFilledCircle(screen, float32(g.enemyX), float32(g.enemyY), 4, color.RGBA{0xff, 0, 0, 0xff}, true)

	for _, b := range g.bullets {
		//vector.DrawFilledCircle(screen, float32(b.x), float32(b.y), 3, color.White, true)
		opts := &ebiten.DrawImageOptions{}
		opts.GeoM.Translate(b.x-3, b.y-3)
		screen.DrawImage(img, opts)
	}

	ebitenutil.DebugPrint(screen, fmt.Sprintf("%.1f", ebiten.CurrentFPS()))
	ebitenutil.DebugPrintAt(screen, samples[g.index].Name, screenWidth-len(samples[g.index].Name)*7, 0)
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

func (g *Game) initializeRunner() {
	opts := &bulletml.NewRunnerOptions[float64]{
		OnBulletFired: func(_ *bulletml.Bullet) bulletml.Bulleter[float64] {
			b := &bullet{}
			g.bullets = append(g.bullets, b)
			return b
		},
		OnBulletVanished: func(b bulletml.Bulleter[float64]) {
			b.(*bullet).vanished = true
		},
		CurrentShootPosition: func() (float64, float64) {
			return g.enemyX, g.enemyY
		},
		CurrentTargetPosition: func() (float64, float64) {
			return g.playerX, g.playerY
		},
	}
	if runner, err := bulletml.NewRunner(samples[g.index].Data, opts); err != nil {
		panic(err)
	} else {
		g.runner = runner
	}

	g.bullets = nil
}

type bullet struct {
	vanished bool
	x, y     float64
}

func (b *bullet) SetX(x float64) {
	b.x = x
}

func (b *bullet) SetY(y float64) {
	b.y = y
}

func main() {
	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowTitle("BulletML Samples")

	game := &Game{
		playerX: screenWidth / 2,
		playerY: screenHeight * 4 / 5,
		enemyX:  screenWidth / 2,
		enemyY:  screenHeight * 1 / 5,
	}

	game.initializeRunner()

	if err := ebiten.RunGame(game); err != nil {
		panic(err)
	}
}

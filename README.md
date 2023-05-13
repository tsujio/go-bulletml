# go-bulletml

A Go implementation of [BulletML](http://www.asahi-net.or.jp/~cs8k-cyu/bulletml/index_e.html)

![Demo](https://github.com/tsujio/go-bulletml/blob/main/sample.gif?raw=true "Demo")

# Usage

## 1. Write BulletML source

The BulletML specifications are [here](http://www.asahi-net.or.jp/~cs8k-cyu/bulletml/bulletml_ref_e.html)

> **Important**
> 
> **go-bulletml and many BulletML libraries (and also the original) run top-level `<action>` elements that have `label` attribute and the value starts with `"top"` as entry points. For example:**
> 
> - `<action label="top">`
> - `<action label="top-1">`

```xml
<?xml version="1.0" ?>
<bulletml type="vertical" xmlns="http://www.asahi-net.or.jp/~cs8k-cyu/bulletml">
    <action label="top">
        <repeat>
            <times>999</times>
            <action>
                <fire>
                    <direction type="sequence">-5</direction>
                    <bullet />
                </fire>
                <repeat>
                    <times>7</times>
                    <action>
                        <fire>
                            <direction type="sequence">45</direction>
                            <bullet />
                        </fire>
                    </action>
                </repeat>
                <wait>2</wait>
            </action>
        </repeat>
    </action>
</bulletml>
```

## 2. Load BulletML data in your game program

```golang
f, err := os.Open("bulletml.xml")
if err != nil {
	panic(err)
}
defer f.Close()

bml, err := bulletml.Load(f)
if err != nil {
	panic(err)
}
```

## 3. Create new runner

```golang
player := &Player{}
enemy := &Enemy{}
bullets := make([]*Bullet)

opts := &bulletml.NewRunnerOptions{
	// Called when new bullet fired
	OnBulletFired: func(bulletRunner bulletml.BulletRunner, _ *bulletml.FireContext) {
		b := &Bullet{
			runner: bulletRunner,
		}
		b.x, b.y = bulletRunner.Position()
		bullets = append(bullets, b)
	},

	// Tell current enemy position
	CurrentShootPosition: func() (float64, float64) {
		return enemy.x, enemy.y
	},

	// Tell current player position
	CurrentTargetPosition: func() (float64, float64) {
		return player.x, player.y
	},
}

runner, err := bulletml.NewRunner(bml, opts)
if err != nil {
	panic(err)
}

enemy.runner = runner
```

## 4. Call runner's Update method in every loop

```golang
if err := enemy.runner.Update(); err != nil {
	panic(err)
}

for _, b := range bullets {
	if err := b.runner.Update(); err != nil {
		panic(err)
	}
	b.x, b.y = b.runner.Position()
}
```

## Full source code

This sample uses [Ebitengine](https://ebitengine.org/), which is a simple Go game engine.

```golang
package main

import (
	"image/color"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"github.com/tsujio/go-bulletml"
)

const (
	screenWidth  = 640
	screenHeight = 480
)

type Player struct {
	x, y float64
}

type Enemy struct {
	x, y   float64
	runner bulletml.Runner
}

type Bullet struct {
	x, y   float64
	runner bulletml.BulletRunner
}

type Game struct {
	player  *Player
	enemies []*Enemy
	bullets []*Bullet
}

func (g *Game) addEnemy(x, y float64) {
	// Open your BulletML file
	f, err := os.Open("bulletml.xml")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	// Load data
	bml, err := bulletml.Load(f)
	if err != nil {
		panic(err)
	}

	enemy := &Enemy{x: x, y: y}

	// Create BulletML runner option
	opts := &bulletml.NewRunnerOptions{
		// Called when new bullet fired
		OnBulletFired: func(bulletRunner bulletml.BulletRunner, _ *bulletml.FireContext) {
			b := &Bullet{
				runner: bulletRunner,
			}
			b.x, b.y = bulletRunner.Position()
			g.bullets = append(g.bullets, b)
		},

		// Tell current enemy position
		CurrentShootPosition: func() (float64, float64) {
			return enemy.x, enemy.y
		},

		// Tell current player position
		CurrentTargetPosition: func() (float64, float64) {
			return g.player.x, g.player.y
		},
	}

	// Create new runner
	runner, err := bulletml.NewRunner(bml, opts)
	if err != nil {
		panic(err)
	}

	// Set runner to enemy
	enemy.runner = runner

	g.enemies = append(g.enemies, enemy)
}

func (g *Game) Update() error {
	// Update enemies
	for _, e := range g.enemies {
		if err := e.runner.Update(); err != nil {
			panic(err)
		}
	}

	_bullets := make([]*Bullet, 0, len(g.bullets))

	// Update bullets
	for _, b := range g.bullets {
		if err := b.runner.Update(); err != nil {
			panic(err)
		}

		// Set updated bullet position
		b.x, b.y = b.runner.Position()

		// Keep bullets only not vanished and within the screen
		if !b.runner.Vanished() &&
			b.x >= 0 && b.x <= screenWidth && b.y >= 0 && b.y <= screenHeight {
			_bullets = append(_bullets, b)
		}
	}

	g.bullets = _bullets

	return nil
}

var img = func() *ebiten.Image {
	img := ebiten.NewImage(6, 6)
	vector.DrawFilledCircle(img, 3, 3, 3, color.RGBA{0xe8, 0x7a, 0x90, 0xff}, true)
	return img
}()

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{0xf5, 0xf5, 0xf5, 0xff})

	for _, b := range g.bullets {
		opts := &ebiten.DrawImageOptions{}
		opts.GeoM.Translate(b.x-3, b.y-3)
		screen.DrawImage(img, opts)
	}
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

func main() {
	ebiten.SetWindowSize(screenWidth, screenHeight)

	game := &Game{
		player: &Player{x: screenWidth / 2, y: screenHeight - 100},
	}

	game.addEnemy(170, 150)
	game.addEnemy(screenWidth-170, 150)

	if err := ebiten.RunGame(game); err != nil {
		panic(err)
	}
}
```

# Extensions of BulletML Specifications

This library contains some extended features of [BulletML specifications](http://www.asahi-net.or.jp/~cs8k-cyu/bulletml/bulletml_ref_e.html).

These features are not standard specifications, so BulletML sources which contain them would not work on other BulletML libraries.

## Loop variables

You can use loop variables in `<repeat>` elements.

- `$loop.index`
    - Zero-based loop index

```xml
<repeat>
    <times>10</times>
    <action>
        <fire>
            <speed>1 + $loop.index</speed>
            <bullet />
        </fire>
    </action>
</repeat>
```

## Math functions

You can use these functions in expressions.

- `sin`
- `cos`

```xml
<direction>sin($loop.index / 10)</direction>
```

# References

- [BulletML](http://www.asahi-net.or.jp/~cs8k-cyu/bulletml/index_e.html)
- [Ebitengine](https://ebitengine.org/)

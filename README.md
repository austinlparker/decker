# decker

A Go engine for slide decks that run in a terminal. Every slide is code
that draws each frame onto a pixel canvas, so slides can have big
antialiased type, block letters, diagrams, characters and animation at any
terminal size, and a talk is just a Go program. It comes with a presenter
view (notes, timer, previews) in a second window, live reload while you
edit, PNG snapshots and contact sheets, and video export.

It's built on Charm's libraries: [Bubble Tea] runs the app, [Lip Gloss]
composes the screen, and [Harmonica] provides springs.

```sh
go get github.com/austinlparker/decker
```

A deck is a `main` package that builds a `decker.Deck` and hands it to
`decker.Main`; see [Anatomy of a deck](#anatomy-of-a-deck). The engine knows
nothing about any one talk's look: each talk brings its colors, typefaces
and slide templates.

## How it draws big type in a terminal

Normally terminal text can only be as big as the terminal's font. The engine
gets around that: every slide paints onto a **pixel canvas** that is shown
with half-block characters (`▀`). The top half of each cell takes one color
and the bottom half another, so a 240×67 terminal becomes a 240×134-pixel
full-color display. Fonts are drawn onto it, sized as a fraction of the
screen, so type is big whatever the terminal's font size.

### Two kinds of type

- **Smooth type** (TrueType or OpenType fonts) is drawn onto the pixel
  canvas with antialiasing. Sizes are fractions of the screen, so it stays
  the same size on stage whatever the terminal font. A smaller terminal font
  means more pixels, so the same text is smoother. The engine ships Space
  Grotesk and JetBrains Mono (`decker.StockFont`); a talk can load its own with
  `decker.LoadFont`.
- **Block letters** (FIGlet fonts) are designed as terminal characters like
  `█` and `▀`, and drawn onto the pixel canvas at a scale that fills the
  space. Use them for titles, section names and impact lines. The engine
  ships five (`decker.BlockShadow`, `BlockSolid`, `BlockSmall`, `BlockHuge`,
  `BlockFancy`); load more with `decker.LoadFigFont`.

Block fonts don't all have every character. For example, ANSI Regular has
no apostrophe, so "Agents aren't users" draws as "AGENTS ARENT USERS". A
font missing other characters is skipped for another.

One idea per slide: a headline and at most a couple of short lines or a
visual. Put the detail in the speaker notes (`Notes`, shown in the presenter
view). Keep titles under about 24 characters so they stay full size.

## Running a deck

Every deck has the same command line, from `decker.Main`:

```sh
go run .                     # present, from slide 1
go run . -dev                # rebuild and reload on every save, staying on the current slide
go run . -presenter          # the presenter view, in a second window
go run . -list               # slide titles and step counts
go run . -snapshot -slide 6 -step 4 -t 2.5 -w 240 -h 67 -png frame.png
go run . -sheet sheet.png -w 682 -h 171 -shrink 8
go run . -video talk.mp4 -fps 30
```

In dev mode, a save starts a rebuild. Dev mode watches every package of the
deck's own module that it imports (with a `go.work` that includes a local
checkout of decker, the engine too). If the build succeeds, the deck
restarts on the slide and step you were on. If it fails, the compiler errors
show in a red panel and the old version keeps running. Press `r` to replay
the current slide's animations.

`-snapshot` renders one frame without a terminal. `-t` is the number of
seconds since the slide appeared; the default is "long after", which shows
the slide fully settled. `-step` and `-slide` are 1-based. `-sheet` renders
every slide at its last step, four across.

`-video` renders the whole deck with its animations and transitions. Each
build step stays on screen for `-hold` seconds (a slide can ask for longer
with its `Hold` field), and a slide's first step also gets the time its
transition takes. Frames come straight from the pixel canvas, so the video is
as sharp as its `-size` allows. It needs `ffmpeg`, and the video has no sound.

## Presenting: set up the terminal

- **Use a small terminal font, full screen.** More cells means more pixels,
  which means smoother type. The deck sets text sizes itself. Decks are
  presented at a 4pt font, 682×171 cells, and checked at 240×67 and 320×90
  too. To see your size, run `tput cols; tput lines`.
- Run it directly in the terminal (Ghostty, kitty, WezTerm, iTerm2). A
  multiplexer like zellij works, but adds latency to animation.
- The deck paints its own background, so your terminal theme doesn't matter.
- Clickers that send page-up/page-down or arrow keys work out of the box.
- It animates at 60 fps (`-fps 30` if the terminal can't keep up).
- The deck writes to the terminal itself rather than through Bubble Tea's
  renderer (`termout.go`): each frame sends only the cells that
  changed, inside synchronized output so the terminal never shows half a
  frame. If the terminal falls behind, frames are dropped, not queued.
  `ctrl+l` redraws the whole screen.
- Big terminals have a lot of cells to redraw, so avoid animations that
  change large areas on every frame (a pulsing full-screen glow, a shaking
  panel). Let things settle once they've made their entrance. Aim for every
  slide to draw in a few milliseconds at 682×171 (`go test -bench
  Frames/682x171` in the deck's directory, with `decktest.Frames`); the
  engine's own share of a live frame is measured by `go test -bench Live`
  here.

## Presenting: notes, timer and next slide

The projector shows only the slides. Speaker notes, progress and a timer are
in the **presenter view**: a second window on your laptop screen, at a normal
font size, that drives the deck.

```sh
go run .              # window 1, on the projector: the deck
go run . -presenter   # window 2, on your screen: the presenter view
```

Start them in either order. The presenter view waits for the deck, and if
the deck quits or restarts (every save in dev mode), it reconnects by itself.
Its keys are the deck's keys, so point the clicker at the presenter window.
The deck's own keys keep working if the presenter view goes away.

The presenter view shows:

- the slide number, title and build step;
- small previews of what's on screen now and what comes next;
- the current slide's notes;
- a 30-minute timer, which starts when you leave slide 1 (or press `t`);
- a pace check: how far ahead or behind an even pace you are, counting
  build steps, so a slide with four builds gets four times a section
  opener's share of the time.

For a different talk length, pass `-length 45m` to the presenter view.

The two windows talk over a Unix socket in the temp directory, named after
the deck. To run two copies of one deck at once, give each pair its own
`-socket /tmp/other.sock`.

## Keys

These work in the deck and in the presenter view:

| Key | Action |
| --- | --- |
| `→` `l` `space` `pgdn` `j` `↓` `enter` | next step / slide |
| `←` `h` `pgup` `k` `↑` `backspace` | previous |
| `]` `[` | next / previous slide, skipping steps |
| `g` `home` / `G` `end` | first / last slide |
| `12g` or `12⏎` | jump to slide 12 |
| `r` | replay the current slide's animations |
| `q` `ctrl+c` | quit (that window only) |

Only in the presenter view: `t` starts or pauses the timer, `T` resets it.

Only in the deck: `n` shows the notes on the projector (a fallback if the
presenter view isn't running), `?` shows help (`esc` closes it) and
`ctrl+l` redraws the screen. In dev mode the deck also has a one-line
footer with the build status and slide counter. The key table is `keys.go`.

## Anatomy of a deck

A talk is a `main` package with four kinds of file:

| File | What's in it |
| --- | --- |
| `main.go` | `func main() { decker.Main(talk()) }`, plus any flags of the talk's own |
| `theme.go` | the palette, the typefaces, and the `decker.Theme` built from them |
| `style.go` | the talk's slide templates: how a title, a statement, a section opener and a bullet build look |
| `slides*.go` | the slides, and `talk()`: the `decker.Deck` with its name, theme and running order |

```go
import "github.com/austinlparker/decker"

func main() { decker.Main(talk()) }

func talk() decker.Deck {
	return decker.Deck{Name: "my-talk", Theme: theme, Slides: []decker.Slide{
		titleSlide(),
		bullets("Why", "notes…", "First reason", "Second reason"),
		mySlide(),
	}}
}
```

### Theme

`decker.Theme` is the part of a talk's look the engine itself uses: the
background painted behind every slide, the colors of the footer, help,
notes, presenter view and wipe transition, the typefaces the stock
components draw in, and an optional `Overlay` drawn over every slide (a
logo or a handle in the corner, like a TV station's bug). Slides reach it as
`c.Theme`. Everything else about the look (title treatment, backdrops,
characters) is the talk's own code in `style.go`.

### Slides

A slide draws everything itself:

```go
func mySlide() decker.Slide {
	return decker.Slide{
		Title:      "Something",           // window title and slide list
		Steps:      2,                     // "next" presses before moving on (build steps)
		Notes:      "say the thing",       // shown in the presenter view
		Transition: decker.TransitionWipe, // Push (default), Dissolve, Wipe, None
		View: func(c decker.Ctx, sc *decker.Scene) {
			p := sc.Px // the pixel canvas; c.PW() × c.PH() pixels

			top := title(c, sc, "Something") // the talk's title template; returns the y below it

			// Big text: size is a fraction of screen height. Fit shrinks it
			// (and wraps it) until it fits the box you give it.
			size, text := c.Theme.Display.Fit("A short, punchy line", c.X(0.84), c.Y(0.3), c.Size(0.14), 0)
			decker.Text{Font: c.Theme.Display, Size: size, Color: c.Theme.Accent, Glow: 0.4,
				FX: decker.RiseIn(c.T, 0.02, size)}.Draw(p, text, c.X(0.08), top)

			if c.Reached(1) { // appears on the second step
				r := c.Unit(0.05) * decker.EaseOutBack(decker.Progress(c.Since(1), 0, 0.4))
				p.Disc(c.X(0.8), c.Y(0.7), r, c.Theme.Accent2, 1)
			}
		},
	}
}
```

`View` runs about 60 times per second. It draws onto `sc`, a scene the engine
has already filled with the theme's background and will show (and release)
when `View` returns, with the theme's `Overlay` on top; a nil `View` is a blank
slide. The `Ctx` it receives holds everything that changes between frames:

- `c.T`: seconds since the slide appeared.
- `c.Step`, `c.StepT`: the current build step, and seconds since it began.
- `c.Reached(n)`, `c.Since(n)`: helpers for "has step n happened" and "how
  long ago".
- Layout in screen fractions: `c.X(0.5)` is the horizontal center,
  `c.Y(0.9)` is near the bottom, `c.Size(0.1)` is type one tenth of the
  screen tall, and `c.Unit(0.02)` is a small length for strokes and gaps.
  Keep text at least `c.Size(decker.MinText)` (`c.SmallText(font)`).
- `c.Theme`: the talk's colors and typefaces.

A frame depends only on `Ctx`, so any moment can be replayed, snapshotted, or
resumed. Never use the clock or `math/rand` in a slide; `decker.Hash01` gives
repeatable noise. If a slide panics, the deck draws the error in place of the
slide, in the pixels, so a video or PNG shows it too.

### Toolbox

Everything here is in package `decker`.

| Want | Use |
| --- | --- |
| Layout | `Rect`: start from `c.Frame()` or `c.Rect(fx, fy, fw, fh)`, then `Inset`, `CutTop`/`CutBottom`/`CutLeft`/`CutRight`, `Rows`/`Cols` (by weight), `Grid`, `Sub` (fractions of the rect) and `Place` (anchor a box inside it); `LerpRect` animates between two |
| Big type | `Text{Font, Size, Color, To (gradient), Glow, Shine, FX, MaxW}.Draw(p, s, x, y)` with `Align`; returns width and height. `DrawMid` centers a line's ink on a y |
| Sizing text to a box | `font.Fit(...)`, `FitAll(...)` for several lines at one size |
| Small labels | `c.SmallText(font)`: the smallest readable size; `Label`, `Chip`, `LineLabel` (text sitting on an arrow) |
| Block letters | `f, lines, scale := FitBlock(s, maxW, maxH, maxLines, gap, fonts...)` picks a font and scale for a pixel box; `Block{Font, Scale, Color, To, Shadow, Drop, Align, Glow, FX}.Draw(p, s, x, y)`; `BlockEffect`s for `Block.FX`: `BlockDecrypt`, `BlockRain`, `BlockBeam`, `BlockSlide`, `BlockType`, `BlockGlitch`, `BlockFade`, combined with `BlockChain` |
| Diagrams | `Panel`, `Arrow`, `CycleDiagram` (numbered ring with a legend), `BulletList`, `SpeechBubble` |
| Unfinished material | `PlaceholderBox` (dashed frame) and `IllustrativeTag` (made-up data) mark what to replace before the talk |
| Letter animations | `GlyphEffect`s for `Text.FX`: `RiseIn`, `DropIn`, `Decode` (scramble), `TypeOn`, `FadeUp`, `Wave`, `Jitter`, combined with `Chain` |
| A moving highlight | `Shine: ShineBand(t, dur, strength)` |
| Shapes | `p.Disc`, `p.Arc` (rings, gauges), `p.Line`, `p.Rect`, `p.RoundRect` (fill or outline), `p.Glow`, `p.VGradient`; `p.Box` and `Coverage` for shapes of your own |
| Pixel art | `p.Art(PixelArt{Rows, Colors}, x, y, scale, alpha, flip)`; return a different frame for a different `t` to animate |
| An overlay on every slide | `Theme.Overlay: func(c Ctx, p *Pixels)`: a logo, a handle or a page tag in a corner that slides leave clear |
| Images | `NewImages(fsys, dir)` over the talk's embedded files, then `images.Draw(p, "shot.png", x, y, w, h, alpha)` |
| Motion | `Ease`, `EaseOutBack`, `EaseInOutCubic`, `Spring` (Harmonica), `Pulse`, `Lerp`, `Progress` |
| Color | `Hex("#FFB000")`, `Mix`, `RGB.Scale` |
| Off-screen drawing | `NewScene(w, h, theme)`, then `Render` it to a string (to `sc.Put` on the slide's own scene) or `Release` it when you've copied what you need |
| Small native terminal text (rarely) | `sc.Text`, `sc.Put`, `sc.Sprite` draw on top of the pixels; videos only have the pixels |

### Tests

[`decktest`](decktest/decktest.go) is the test suite every deck runs. A talk's `talk_test.go`:

```go
func TestSlides(t *testing.T)      { decktest.Slides(t, talk()) }
func TestGolden(t *testing.T)      { decktest.Golden(t, talk(), "testdata/golden.txt") }
func BenchmarkFrames(b *testing.B) { decktest.Frames(b, talk()) }
```

`Slides` renders every slide at every build step, at three sizes and four
moments, and fails on panics. `Golden` hashes every frame of every step at eight moments and
three sizes, and fails if any of them changed: after changing a slide on
purpose, record it with `UPDATE_GOLDEN=1 go test -run Golden`. That makes
the engine safe to change: an engine change that moves one pixel of any
talk fails that talk's test.

## Where things live

Changing the engine itself? Read [AGENTS.md](AGENTS.md) first: the
invariants, the golden tests, and a recipe for each kind of addition.

| File | What's in it |
| --- | --- |
| `deck.go`, `slide.go`, `ctx.go` | `Deck`, `Slide`, and `Ctx` with its layout in screen fractions |
| `layout.go` | `Rect`: boxes cut from the canvas for layout |
| `cli.go` | `Main`: the command line (live, dev, presenter, list, snapshot, sheet, video) |
| `theme.go`, `color.go` | `Theme`; `RGB`, `Hex`, `Mix` |
| `draw.go` | stock components: `Panel`, `Arrow`, `Label`, `Chip`, `CycleDiagram`, `BulletList`… |
| `font.go`, `fit.go`, `text.go`, `coverage.go`, `memo.go`, `fonts/` | smooth type: font loading and glyphs, fitting and wrapping, drawing with glow and gradients, coverage masks, cached fits |
| `figlet.go`, `block.go`, `blockfit.go`, `blockglyph.go`, `fonts/figlet/` | block letters: FIGlet font loading, drawing, fitting to a box, block-character glyphs |
| `effects.go`, `blockfx.go` | letter animations: `GlyphEffect` for `Text`, `BlockEffect` for `Block` |
| `pixels.go`, `pixelart.go`, `image.go` | the pixel canvas and shapes, pixel art, images |
| `anim.go` | easing, springs, noise |
| `scene.go`, `grid.go`, `pool.go` | combine the pixel canvas and character layer into terminal cells; reused frame buffers |
| `render.go` | a slide's frame: the scene `View` draws on, the overlay, panics caught |
| `transition.go` | slide transitions: each mixes two finished scenes, for the terminal and video alike |
| `model.go`, `keys.go`, `termout.go`, `dev.go` | the app: navigation, the key table, writing frames, dev reload |
| `presenter.go`, `link.go`, `preview.go` | the presenter view, the socket link to the deck, slide previews |
| `png.go` | PNG snapshots and contact sheets, painted from cell grids |
| `video.go` | rendering a deck to a video |
| `decktest/` | the test suite for decks |

Space Grotesk and JetBrains Mono are under the SIL Open Font License; see
`fonts/*-OFL.txt`. The FIGlet fonts come from xero/figlet-fonts, which
states no license; see `fonts/figlet/README.md`.

[Bubble Tea]: https://github.com/charmbracelet/bubbletea
[Lip Gloss]: https://github.com/charmbracelet/lipgloss
[Harmonica]: https://github.com/charmbracelet/harmonica

# Decker guide

[README](../README.md) · [CLI reference](cli.md) · [API reference](https://pkg.go.dev/github.com/austinlparker/decker)

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
  ships five BSD-licensed Spleen defaults (`decker.BlockShadow`,
  `BlockSolid`, `BlockSmall`, `BlockHuge`, `BlockFancy`) and five optional
  OFL display-font conversions. Choose a bundled face with
  `decker.StockFigletFont`, or supply your own with `decker.LoadFigletFont` or
  `decker.ParseFigletFont`. See the [font inventory](../fonts/figlet/README.md).

Stock Spleen block fonts preserve lowercase and all printable ASCII
punctuation, so "Agents aren't users" retains its case and apostrophe.
Custom block fonts may omit glyphs: `FitBlock` skips faces missing a needed
character and removes unsupported quote marks with `DropQuotes`. The
FIGlet loader currently loads ASCII input glyphs only.

One idea per slide: a headline and at most a couple of short lines or a
visual. Put the detail in the speaker notes (`Notes`, shown in the presenter
view). Keep titles under about 24 characters so they stay full size.

### Choosing and loading fonts

`decker.StockFigletFontNames()` lists all ten bundled block fonts. The five
`Block*` defaults above retain their Spleen designs. These additional
faces are converted from licensed outline fonts to pixel blocks:

| Pass to `StockFigletFont` | Source design | Style |
| --- | --- | --- |
| `"Decker Square"` | Russo One | Square display lettering with true lowercase |
| `"Decker Sign"` | Bungee | Chunky sign lettering, uppercase-shaped lowercase |
| `"Decker Geometric"` | Rubik Mono One | Very heavy, wide geometric lettering, uppercase-shaped lowercase |
| `"Decker Stencil"` | Black Ops One | Angular stencil lettering with true lowercase |
| `"Decker Slab"` | Alfa Slab One | Heavy slab serifs with true lowercase |

The older `FigFont`, `LoadFigFont`, `ParseFigFont`, `StockFigFont` and
`StockFigFontNames` names remain available as deprecated compatibility
aliases. Use the `FigletFont` spellings in new code.

Load fonts once while building the deck, then capture them in slide
functions and pass them through `Block.Font` or `FitBlock`:

```go
headline := decker.StockFigletFont("Decker Sign")

slide := decker.Slide{
    Title: "Hello",
    View: func(c decker.Ctx, sc *decker.Scene) {
        f, lines, scale := decker.FitBlock("Hello, Go!", c.X(0.8), c.Y(0.3), 1, 0,
            headline, decker.BlockSolid)
        decker.Block{Font: f, Scale: scale, Color: c.Theme.Text, Drop: c.Theme.Muted}.
            Draw(sc.Px, strings.Join(lines, "\n"), c.X(0.1), c.Y(0.2))
    },
}
```

This snippet uses `strings.Join` from the Go standard library. A drop shadow
is a drawing option rather than a separate font. Wider, more detailed
faces need more space; `FitBlock` accepts your preferred fonts in order
and scales or wraps them to the given box.

Consumers can bundle their own `.flf`, `.ttf` and `.otf` files with `embed`.
For example, these declarations belong in the talk's package, outside any
frame function:

```go
import (
    "embed"
    "github.com/austinlparker/decker"
)

//go:embed fonts/*
var fontFiles embed.FS

var customBlock = decker.LoadFigletFont(fontFiles, "fonts/Custom.flf")
var customDisplay = decker.LoadFont(fontFiles, "fonts/Custom-Bold.ttf")
```

Assign `customBlock` to `Block.Font` or pass it to `FitBlock`; assign
`customDisplay` to `Text.Font` or a theme's `Display`, `Body` or `Mono`.
For files on disk, use `os.DirFS("fonts")` and paths relative to that
directory, such as `decker.LoadFigletFont(os.DirFS("fonts"), "Custom.flf")`.
Both loaders panic on missing or broken fonts, which suits fixed assets.
For uploaded files or configuration that needs error handling, read the
bytes and use `decker.ParseFigletFont("Custom", data)` or
`decker.ParseFont(data)`; both return a font and an error. A parsed font
does not need registration in a global catalog to be drawn.

Custom FIGlet fonts need a `flf2a` header and the 95 printable ASCII input
glyph slots. Decker paints selected Unicode blocks, shades and box-drawing
strokes inside those glyphs; ordinary ASCII-art strokes such as `/`, `_`
and `|` are not painted by `Block`. Native Unicode input glyphs and TOIlet
`tlf2a` headers are not supported. Check a custom font's actual rendering
and `Has` coverage before using it. Keep each supplied font's own license
and attribution with your talk; the bundled conversions retain their
[OFL and BSD notices](../fonts/README.md).

## Running a deck

Every deck has the same command line, from `decker.Main`. Run these commands
from the talk's `main` package. See the [CLI reference](cli.md) for all flags
and defaults:

```sh
go run .                     # present, from slide 1
go run . -dev                # rebuild and reload on every save, staying on the current slide
go run . -presenter          # the presenter view, in a second window
go run . -presenter -presentation-font-size 5 # p opens the deck at 5pt in Ghostty
go run . -list               # slide titles, step counts and sections
go run . -review review      # check every build at three sizes for clipped or lost content
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
seconds since the slide appeared; the default is `decker.Settled` (1000), which shows
the selected step fully settled. Omitting `-step` selects the first step;
to capture the completed slide, pass its final step explicitly. `-step` and `-slide` are 1-based. `-sheet` renders
every slide at its last step, four across.

`-review DIR` checks every build of every slide, settled, at 240×67, 320×90
and 682×171, and lists what is lost or hard to read: Code lines clipped,
Table rows dropped, text off the canvas or below the readable size, a
panic. It writes `DIR/index.md` and `DIR/report.json`, and exits non-zero
when it finds an error. See [Reviewing a deck](#reviewing-a-deck).

`-video` renders the whole deck with its animations and transitions. Each
build step stays on screen for `-hold` seconds (a slide can ask for longer
with its `Hold` field), and a slide's first step also gets the time its
transition takes. Frames come straight from the pixel canvas, so the video is
as sharp as its `-size` allows. It needs `ffmpeg` with the `libx264` encoder. Video contains only the pixel
canvas, so native terminal text from `Scene.Text`, `Put` and `Sprite` is
omitted. The video has no sound, and export replaces an existing output file.

## Presenting: set up the terminal

- **Use a small terminal font, full screen.** More cells means more pixels,
  which means smoother type. The deck sets text sizes itself. Decks are
  presented at a 4pt font, 682×171 cells, and checked at 240×67 and 320×90
  too. To see your size, run `tput cols; tput lines`.
- Run it directly in the terminal (Ghostty, kitty, WezTerm, iTerm2). A
  multiplexer like zellij works, but adds latency to animation.
- The deck paints its own background, so your terminal theme doesn't matter.
- Clickers that send page-up/page-down or arrow keys work out of the box, and
  a "blank" button (`b` or `.`) blanks the screen to black.
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
go run . -presenter   # on your screen: notes, timer and previews
```

In Ghostty 1.3+ on macOS, press `p` to open the deck in a new window at a
4pt font. Move that window to the projector. The presenter stays at its
normal font size, and focus returns to it after the deck window opens.
Use `-presentation-font-size 5` for a different deck-window font size.
macOS may ask for permission to automate Ghostty the first time.

The new window runs the same executable, even when you used `go run`, with
the same working directory, socket, and talk-specific command-line flags.
It starts at `-slide`/`-step`, or the last reported position after a
disconnect. Repeated `p` presses do not open duplicate windows; after the
deck closes, `p` can reopen it. Quitting the presenter leaves the deck
running. Launch failures appear in the presenter and can be retried with `p`.

Other terminals and platforms can start the two windows manually:

```sh
go run .              # window 1, on the projector: the deck
go run . -presenter   # window 2, on your screen: the presenter view
```

Start them in either order. The presenter view waits for the deck, and if
the deck quits or restarts (every save in dev mode), it reconnects by itself.
Its keys are the deck's keys, so point the clicker at the presenter window.
The deck's own keys keep working if the presenter view goes away.

The presenter view shows:

- the slide number, title and build step, and a `BLANK` tag while the deck is
  blanked (`b`, `w`);
- small previews of what's on screen now and what comes next;
- the current slide's notes;
- a 30-minute timer, which starts when you leave slide 1 (or press `t`);
- a pace check: how far ahead or behind an even pace you are, counting
  build steps, so a slide with four builds gets four times a section
  opener's share of the time.

Ghostty and kitty show full-resolution slide images automatically using
the Kitty graphics protocol. Other terminals, and sessions inside tmux or
zellij, use half-block previews. Use `-previews image` to force image
previews, or `-previews cells` to force cells. Images render at the deck's
actual dimensions and settled build step; while an image loads, its cell
preview remains visible. Resizing either window refreshes the images.

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
| `b` `.` | blank the screen to black; again to resume |
| `w` `,` | blank the screen to white; again to resume |
| `q` `ctrl+c` | quit (that window only) |

A blank screen is pure black or white over the whole screen, footer included,
like PowerPoint's B and W (many clickers' "blank" button sends `b` or `.`).
Pressing the same key again brings the slide back, and so does any
navigation key, which then also does its move. The other blank key switches
color. The slide's clock keeps running underneath, so animations finish while
it is up. Pressed in the presenter view, the keys blank the deck, and the
presenter view shows `BLANK` in its header while keeping the notes and
previews. Blanking closes the notes and help.

Only in the presenter view: `p` opens the deck in a new Ghostty window
(macOS), `t` starts or pauses the timer, `T` resets it.

Only in the deck: `n` shows the notes on the projector (a fallback if the
presenter view isn't running), `?` shows help (`esc` closes it) and
`ctrl+l` redraws the screen. In dev mode the deck also has a one-line
footer with the build status and slide counter. The key table is [`keys.go`](../keys.go).

## Anatomy of a deck

Start with the complete [hello example](../examples/hello/main.go). As a talk grows, split its `main` package into four kinds of file:

| File | What's in it |
| --- | --- |
| `main.go` | `func main() { decker.Main(talk()) }`, plus any flags of the talk's own |
| `theme.go` | the palette, the typefaces, and the `decker.Theme` built from them |
| `style.go` | the talk's slide templates: how a title, a statement, a section opener and a bullet build look |
| `slides*.go` | the slides, and `talk()`: the `decker.Deck` with its name, theme and running order |

The next snippets illustrate a larger talk's structure; `theme`,
`titleSlide`, `bullets` and `title` are your own templates. For a program
you can run unchanged, use the [hello example](../examples/hello/main.go).

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
logo or a handle in the corner, like a TV station's bug). An optional `Syntax`
(`*SyntaxColors`) sets the colors `Code` highlights with; left nil, they are
derived from the palette. Slides reach it as `c.Theme`. Everything else about
the look (title treatment, backdrops, characters) is the talk's own code in `style.go`.

### Slides

A slide draws everything itself:

```go
func mySlide() decker.Slide {
	return decker.Slide{
		Title:      "Something",                     // window title and slide list
		Steps:      2,                               // two build states: Ctx.Step 0 and 1
		Notes:      "say the thing",                 // shown in the presenter view
		Section:    "Part one",                      // chapter; later slides inherit it until one sets another
		Transition: decker.TransitionWipe.Over(0.6), // Push (default), Dissolve, Wipe, Morph, Fade, FadeThrough, Cover, Uncover, Split, Iris, Zoom, Pixelate, Glitch, None; Over sets seconds, From a side
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
- `c.Step`, `c.StepT`: the current build step (0-based), and seconds since it began.
- `c.Reached(n)`, `c.Since(n)`: helpers for "has step n happened" and "how
  long ago".
- Layout in screen fractions: `c.X(0.5)` is the horizontal center,
  `c.Y(0.9)` is near the bottom, `c.Size(0.1)` is type one tenth of the
  screen tall, and `c.Unit(0.02)` is a small length for strokes and gaps.
  Keep text at least `c.Size(decker.MinText)` (`c.SmallText(font)`).
- `c.Index`, `c.Count`, `c.Section`: the slide's 0-based position, the number
  of slides, and its section (`Slide.Section`, inherited from the nearest
  slide before it that sets one). They exist for `Theme.Overlay`: a page
  number, a progress bar or a chapter name on every slide.
- `c.Theme`: the talk's colors and typefaces.

A frame depends only on `Ctx`, so any moment can be replayed, snapshotted, or
resumed. Never use the clock or `math/rand` in a slide; `decker.Hash01` gives
repeatable noise. If a slide panics, the deck draws the error in place of the
slide, in the pixels, so a video or PNG shows it too.

### Magic move

A slide can hand the engine *elements* instead of drawing them itself:
`sc.Place(key, rect, draw)` records one, and the engine calls `draw(p, rect)`
after `View` returns, above everything `View` drew. On a slide that enters
with `decker.TransitionMorph`, an element whose key the slide before it also
placed glides from its old rect to its new one, cross-fading from the old
`draw` to the new. Elements on only one side fade out or in, and the rest of
both slides cross-fades. A morph takes `decker.MorphDuration` (0.8s), longer
than the other transitions' 0.45s; `TransitionMorph.Over(1.2)` changes it.

```go
sc.Place("headline", head, func(p *decker.Pixels, r decker.Rect) {
	size, s := c.Theme.Display.Fit("Agents", r.W, r.H, c.Size(0.3), 0)
	decker.Text{Font: c.Theme.Display, Size: size, Color: c.Theme.Accent}.Draw(p, s, r.X, r.Y)
})
```

`draw` should size itself from `r` (so it looks right at every rect in
between) and stay inside it, give or take a glow. During a morph, drawing
more than a tenth of the screen's height outside the rect is cut off. A
transition is how a slide *enters*, so going back plays the transition of
the slide you return to: give both slides `TransitionMorph` to morph both
ways.

### Element animations

`Text` animates letter by letter; everything else (a panel, an image, a
diagram) animates as a whole through a `Composite`: its drawing goes onto a
layer, and the layer is put back faded, moved, scaled about a pivot and
clipped. Animations are pure functions of `t`, the seconds since they start,
and return a `Composite`; they come in three kinds. Entrances start hidden and
end untouched, exits start untouched and end hidden, and emphasis (`Grow`,
`Shake`, `Dim`) leaves the element as it was. `AppearAt` turns the build
steps an element comes and goes at into this frame's `Composite`, including
"hidden until its step".

```go
r := c.Rect(0.1, 0.3, 0.3, 0.3)
fx := decker.AppearAt(c, 1, 3, // in at step 1, out at step 3
	func(t float64) decker.Composite { return decker.FlyIn(c, t, 0.5, decker.DirLeft, 0.1) },
	func(t float64) decker.Composite { return decker.FadeOut(t, 0.4) },
).Then(decker.Grow(c.Since(2), 0.4, 1.1)) // a pulse at step 2
fx.Draw(p, r, func(p *decker.Pixels) {
	decker.Panel(c, p, r.X, r.Y, r.W, r.H, "hello", c.Theme.Panel, c.Theme.Accent, c.Theme.Text, 1)
})
```

`bounds` is where the element lives: only it and a margin of a tenth of the
screen's height around it (for a glow) is put on a layer, and `Trim` is a
fraction of it. An identity composite draws straight onto the canvas, and
a fade or wipe alone costs one copy of the region. A move or scale draws the
element twice (on black and on white, which tells the layer's coverage) and
resamples it, so `draw` must be pure, and an element moved off whole pixels is
a little soft. Additive light (`p.Glow`) in a moved element blends as a screen
rather than adding. Moving a large element every frame is the kind of full
region redraw the terminal-setup advice warns about: let it settle.

### Toolbox

Everything here is in package `decker`.

| Want | Use |
| --- | --- |
| Layout | `Rect` (or `NewRect(x, y, w, h)` in pixels): start from `c.Frame()` or `c.Rect(fx, fy, fw, fh)`, then `Inset`, `CutTop`/`CutBottom`/`CutLeft`/`CutRight`, `Rows`/`Cols` (by weight), `Grid`, `Sub` (fractions of the rect) and `Anchor` (a box of a given size inside it); `LerpRect` animates between two |
| Transitions | `Slide.Transition`: `TransitionPush` (default), `Cover`, `Uncover`, `Wipe`, `Split`, `Fade`, `FadeThrough`, `Dissolve`, `Iris`, `Zoom`, `Pixelate`, `Glitch`, `Morph`, `None`. `.Over(secs)` sets the time; `.From(DirLeft / DirRight / DirUp / DirDown)` sets the side of Push, Cover, Uncover and Wipe (and the axis of Split); going back plays from the opposite side. Vertical moves go in whole cell rows |
| Magic move | `sc.Place(key, rect, draw)` on both slides, `Transition: TransitionMorph` on the second; see [Magic move](#magic-move) |
| Big type | `Text{Font, Size, Color, To (gradient), Glow, Shine, FX, MaxW}.Draw(p, s, x, y)` with `Align`; returns width and height. `DrawMid` centers a line's ink on a y |
| Mixed styles in a line | `Rich{Font, Size, Color, Glow, FX, MaxW}.Draw(p, spans, x, y)` with `[]Span{Text, Font, Color, Underline, Strike, Mark}`: bold a word, color a keyword, highlight, inline code, one baseline, wrapping across spans; ``ParseSpans("*bold* _muted_ `code` {accent:word}", theme)`` builds spans from light markup (see [Rich text](#rich-text)); `rich.Fit(spans, w, h, maxSize)` sizes them to a box |
| Sizing text to a box | `font.Fit(...)`, `FitAll(...)` for several lines at one size |
| Checking what fits | `c.Fits(what, rect, w, h)` reports a block that doesn't fit its rect to the review; `c.Report(severity, code, rect, msg)` any other problem; `c.Reviewing()` guards costly checks; `Code.Measure(c, rect)` says what a code block shows before drawing it. See [Reviewing a deck](#reviewing-a-deck) |
| Small labels | `c.SmallText(font)`: the smallest readable size; `Label`, `Chip`, `LineLabel` (text sitting on an arrow) |
| Block letters | `f, lines, scale := FitBlock(s, maxW, maxH, maxLines, gap, fonts...)` picks a font and scale for a pixel box; `Block{Font, Scale, Color, To, Shadow, Drop, Align, Glow, FX}.Draw(p, s, x, y)`; `BlockEffect`s for `Block.FX`: `BlockDecrypt`, `BlockRain`, `BlockBeam`, `BlockSlide`, `BlockType`, `BlockGlitch`, `BlockFade`, combined with `BlockChain` |
| Diagrams | `Panel`, `Arrow`, `CycleDiagram` (numbered ring with a legend), `BulletList`, `SpeechBubble`; `Timeline{Items, FirstStep, Vertical}.Draw(c, p, rect)` (milestones on a line, one per step) and `Process{Steps, FirstStep}.Draw(c, p, rect)` (a row of chevrons, one per step) |
| Joining boxes | `Connector{From, To Rect, Route, Head, Tail, Width, Color, Dashed, Label, Prog}.Draw(c, p)`: a line, an elbow (`RouteElbow`) or a curve (`RouteCurved`) between two rects, leaving and arriving at the sides that face each other (`FromSide`/`ToSide` to force them), with `HeadArrow`/`HeadDot` ends flush on the edge; `Prog` draws it on, so `Ease(c.Since(step), 0.6)` animates it |
| Code | `Code{Source, Lang, LineNumbers, Title, Focus, FirstStep, Diff, Size}.Draw(c, p, rect)`: syntax-highlighted (chroma) in `Theme.Mono` on a plate, sized to fit the rect; `Focus` is one `LineRange{From, To}` per step from `FirstStep`, dimming the other lines behind a highlight bar that glides between ranges; `Diff` reads a unified diff. Colors from `Theme.Syntax`, or derived from the theme |
| Charts | `BarChart{Labels, Values or Series, Max, Horizontal, ShowValues, Step}`, `LineChart{Labels, Series, Names, Min, Max, Points, Step}`, `DonutChart{Labels, Values, Thickness, Center, Step}`, each with `.Draw(c, p, rect)`: they grow in on their step, take colors from `Theme.Series` (`t.SeriesColor(i)`; default Accent, Accent2, Good, Warn, Muted) and survive empty, zero, negative and NaN data. `Sparkline(c, p, rect, values, col, prog)` is a tiny inline line |
| A counting number | `Stat{Value, Prefix, Suffix, Decimals, Label, Step, Duration}.Draw(c, p, rect)`: counts up from 0 on its step, at a steady width |
| Tables | `Table{Header, Rows, Weights, Align, Zebra, Rules, FirstStep, Reveal, Highlight, Walk}.Draw(c, p, rect)`: one text size for every cell, ragged rows, reveal by row or column over steps (`TableRevealRows`, `TableRevealCols`), and a highlight that can `Walk` down the rows |
| Unfinished material | `PlaceholderBox` (dashed frame) and `IllustrativeTag` (made-up data) mark what to replace before the talk |
| Letter animations | `GlyphEffect`s for `Text.FX` and `Rich.FX`: `RiseIn`, `DropIn`, `Decode` (scramble), `TypeOn`, `FadeUp`, `Wave`, `Jitter`, combined with `Chain` |
| A moving highlight | `Shine: ShineBand(t, dur, strength)` |
| Shapes | `p.Disc`, `p.Arc` (rings, gauges), `p.Line`, `p.Rect`, `p.RoundRect` (fill or outline), `p.Glow`, `p.VGradient`; `p.Ellipse` (fill or outline), `p.Polygon` (concave shapes, even-odd), `p.Polyline` (one blend, so joints don't show at partial alpha), `p.Bezier` (cubic, drawn on to a fraction of its length), `p.DashedLine`, `p.LinearGradient` and `p.RadialGradient` (fill a rect or rounded rect); `p.Box` and `Coverage` for shapes of your own |
| Pixel art | `p.Art(PixelArt{Rows, Colors}, x, y, scale, alpha, flip)`; return a different frame for a different `t` to animate |
| An overlay on every slide | `Theme.Overlay: func(c Ctx, p *Pixels)`: a logo, a handle or a page tag in a corner that slides leave clear |
| Page number, progress, section | `PageNumber` ("12 / 40"), `ProgressBar` (share of slides shown), `c.Section`: for `Theme.Overlay`, from `c.Index` and `c.Count` |
| Images | `NewImages(fsys, dir)` over the talk's embedded files, then `images.Draw(p, "shot.png", x, y, w, h, alpha)` (fit inside the box) or `images.DrawCover(...)` (fill the box, crop the overflow); PNG transparency is kept |
| Element animations | `AppearAt(c, enter, exit, in, out)` builds a `Composite` from "in at step n, out at step m"; entrances `FadeIn`, `FlyIn`, `ZoomIn`, `Pop`, `WipeIn`, exits `FadeOut`, `FlyOut`, `ZoomOut`, `WipeOut`, emphasis `Grow`, `Shake`, `Dim`; join with `Then` or `Combine`, draw with `fx.Draw(p, bounds, draw)`; see [Element animations](#element-animations) |
| Layers | `Composite{Alpha, DX, DY, Scale, PivotX, PivotY, Trim}.Draw(p, bounds, draw)` draws a group on a pooled layer and fades, moves, scales and clips it as one; `Identity()` is the no-cost default, `Clipped(bounds, clip)` clips to a rect |
| Motion | `Ease`, `EaseOutBack`, `EaseInOutCubic`, `EaseInQuad`/`EaseOutQuad`/`EaseInOutQuad`, `EaseInCubic`, `EaseOutExpo`/`EaseInOutExpo`, `EaseOutElastic`, `EaseOutBounce`, `CubicBezier(x1, y1, x2, y2)` (CSS-style), `Spring` (Harmonica), `Pulse`, `Lerp`, `Progress` |
| Color | `Hex("#FFB000")`, `Mix`, `RGB.Scale` |
| Off-screen drawing | `NewScene(w, h, theme)`, then `Render` it to a string (to `sc.Put` on the slide's own scene) or `Release` it when you've copied what you need |
| Small native terminal text (rarely) | `sc.Text`, `sc.Put`, `sc.Sprite` draw on top of the pixels; videos only have the pixels |

### Rich text

`Text` draws one font and color. `Rich` takes `[]Span` and lets each span differ, on a shared baseline; it wraps at spaces across span boundaries and counts `FX` glyph indexes as `Text` does. `Rich.Fit` finds the largest size at which spans fill a box, remembered between frames like `Font.Fit`.

```go
spans := decker.ParseSpans("Call *Draw* with `[]Span`, {accent:not} a string.", c.Theme)
r := decker.Rich{Font: c.Theme.Body, Color: c.Theme.Text}
r.Size, r.MaxW = r.Fit(spans, w, h, c.Size(0.2)), w
r.Draw(p, spans, x, y)
```

`ParseSpans` markup, with the theme supplying fonts and colors:

| Write | Get |
| --- | --- |
| `*bold*` | the Display face (a theme has no bold cut) |
| `_muted_` | `Theme.Muted` |
| `` `code` `` | `Theme.Mono` on a plate; markup inside is literal |
| `{accent:word}` | a color: `text`, `muted`, `faint`, `accent`, `accent2`, `warn`, `good`, `#rrggbb` or `#rgb` |
| `{u:word}`, `{s:word}` | underline, strikethrough |
| `{mark:word}` | highlighter pen: accent plate, background-colored ink |
| `\*` | a literal marker; `\\`, `\_`, ``\` ``, `\{` and `\}` likewise |

Markers toggle wherever they sit, so `*bo*ld` bolds half a word; write `snake\_case`. Build `Span`s by hand for anything else (`Underline`, `Strike`, `Mark`, any `Font` or `Color`).

### Tests

[`decktest`](../decktest/decktest.go) is the test suite every deck runs. A talk's `talk_test.go`:

```go
package main

import (
	"testing"

	"github.com/austinlparker/decker/decktest"
)

func TestSlides(t *testing.T)      { decktest.Slides(t, talk()) }
func TestReview(t *testing.T)      { decktest.Review(t, talk()) }
func TestGolden(t *testing.T)      { decktest.Golden(t, talk(), "testdata/golden.txt") }
func BenchmarkFrames(b *testing.B) { decktest.Frames(b, talk()) }
```

`Slides` renders every slide at every build step, at three sizes and four
moments, and fails on panics in a View, a placed element or the overlay.
`Review` fails on every error `-review` would report (see below), so a slide
that starts clipping fails the build. `Golden` hashes every frame of every step at eight moments and
three sizes, and fails if any of them changed: after changing a slide on
purpose, record it with `UPDATE_GOLDEN=1 go test -run Golden`. That makes
the engine safe to change: an engine change that moves one pixel of any
talk fails that talk's test.


### Reviewing a deck

A slide that compiles and renders can still lose half its content: a code
block taller than its rect, a table with more rows than fit, a line of text
running off the bottom. Each only shows up at some sizes, and often only at
an intermediate build. The review finds them:

```sh
go run . -review review
```

```
11.2  Collector config  320x90  error    code-clipped  Code "otel.yaml": 5 of 10 lines visible (needs 236px tall, has 118px) at 12px, the smallest readable size
7.3   Waterfall         240x67  warning  text-small    BarChart: 4 texts at 7px, such as Text "db.query"; the smallest readable size here is 9px
1 error and 1 warning in 41 builds of 12 slides at 240x67, 320x90, 682x171.
```

It draws every build of every slide, settled, at each size, and listens
while the frames are drawn (the frames are the ones the deck shows). It also
compares frames: a build that looks the same as the one before it, a slide
that draws differently when drawn again, a morph with nothing to move. Each
issue has a severity, a stable code, the rect it is about and a message with
the numbers:

| Code | Severity | Means |
| --- | --- | --- |
| `code-clipped` | error | a `Code` block shows only some of its lines or columns |
| `table-rows-dropped` | error | a `Table` left out rows that don't fit, even at the smallest readable size |
| `text-offcanvas` | error, or a warning for the tail of a letter | `Text`, `Rich` or `Block` ink runs past an edge of the canvas |
| `overflow` | error | a block needs more room than its rect: a `Timeline`, `Process` step, `BulletList`, cycle legend, or a slide's own `c.Fits` |
| `panic` | error | the View, a placed element or the overlay panicked |
| `impure` | error | the slide draws differently the second time with the same `Ctx`: it keeps state between frames or reads the clock |
| `text-small` | warning | text below the smallest readable size (`c.SmallText`); a component's text counts once for the component |
| `table-cell-cut` | warning | `Table` cells shortened with "..." |
| `overlap` | warning | two elements (blocks of text, stock components, placed elements) ink the same pixels |
| `overlay-collision` | warning | `Theme.Overlay` draws over something the slide drew |
| `step-unchanged` | warning | a build looks the same as the one before it, settled and while it enters: `Steps` is one too many, or a `c.Reached` names the wrong step |
| `morph-unmatched` | warning | a slide enters with `TransitionMorph` but shares no `Place` key with the slide before, so it cross-fades |
| `code-title-hidden`, `code-title-cut` | warning, note | a `Code` block's title tab has no room, or was shortened |
| `never-settles` | note | a tenth of the frame or more keeps changing after the slide settles, which the terminal must redraw every frame |

`-sizes 240x67,682x171` changes the sizes, `-slide` and `-until` limit the
slides, and `-strict` fails on warnings too. The report is also written to
`review/index.md` (by slide and build) and `review/report.json`, whose
slides and builds are 1-based like the command line, with pictures:

- `review/frames/11-2-320x90.png` for each build with issues: the frame,
  scaled up, with a faint outline around everything it drew and a numbered
  box around each issue, listed under it. `-frames all` writes every build,
  `-frames none` none.
- `review/sheet-320x90.png` per size: every build at a glance, each framed
  in the color of its worst issue (red errors, amber warnings, blue notes).
  An intermediate build that clips shows up here even when the last one is
  fine.

The images come from the pixel canvas, so `Scene.Text`, `Put` and `Sprite`
characters are not in them. For one frame while you work on it,
`-snapshot -png frame.png -bounds` draws the same outlines and boxes.

A slide that means to have an issue says so: `Allow: []string{"text-offcanvas"}`
on a headline that bleeds off the edge on purpose.

Drawing of your own reports the same way. `c.Fits(what, rect, w, h)` returns
whether a `w`×`h` block fits `rect`, and when it doesn't, records an
`overflow` error naming `what`; `c.Report` records anything else. Both do
nothing while presenting.

```go
lay := layoutWaterfall(spans, r) // your own layout: pure, from the rect
if !c.Fits("waterfall", r, lay.W, lay.H) {
	lay = layoutWaterfall(spans[:8], r) // drop the tail rather than clip it
}
```

`Code.Measure(c, rect)` returns how a code block fits before it draws:
the size its text is set at, the plate, the lines and columns that show
(`Shown`, `ShownCols`), and the plate it would need (`NeedW`, `NeedH`).

[Bubble Tea]: https://github.com/charmbracelet/bubbletea
[Lip Gloss]: https://github.com/charmbracelet/lipgloss
[Harmonica]: https://github.com/charmbracelet/harmonica

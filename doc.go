// Package decker is a terminal slide-deck engine: a talk is a Go program, and
// every slide is a function that draws each frame onto a pixel canvas.
//
// A [Deck] holds a [Theme] and a list of slides. Each [Slide] has a View, a
// function that draws one frame from a [Ctx] onto a [Scene], and [Main] gives
// the deck its command line: present, presenter view, dev reload, a review of
// every build for clipped or unreadable content, snapshots, contact sheets,
// handouts and video.
// The presenter can open a deck window in Ghostty on macOS with its own
// terminal font size; Ghostty and kitty also support full-resolution previews.
//
// A minimal talk loads its fonts once and draws from the frame's context:
//
//	func main() {
//		theme := &decker.Theme{
//			Background: decker.Hex("#101820"),
//			Text:       decker.Hex("#F0F0F0"),
//			Display:    decker.StockFont("SpaceGrotesk-Bold"),
//			Body:       decker.StockFont("SpaceGrotesk-Medium"),
//			Mono:       decker.StockFont("JetBrainsMono-ExtraBold"),
//		}
//		decker.Main(decker.Deck{Name: "hello", Theme: theme, Slides: []decker.Slide{{
//			Title: "Hello",
//			View: func(c decker.Ctx, sc *decker.Scene) {
//				decker.Text{Font: c.Theme.Display, Size: c.Size(0.15), Color: c.Theme.Text}.
//					Draw(sc.Px, "Hello", c.X(0.08), c.Y(0.2))
//			},
//		}}})
//	}
//
// Import github.com/austinlparker/decker from a separate main package. See the
// [hello example] for a complete program with animation and build steps, the
// [guide] for slide authoring, and the [CLI reference] for flags and exports.
// This checkout requires Go 1.27.0 or later and uses Unix signals and process
// replacement. Video export additionally requires ffmpeg with libx264.
//
// # Pixel canvas
//
// A slide paints into [Pixels], a true-color framebuffer shown with
// half-block characters, so each terminal cell is two pixels tall and a
// 240x67 terminal is a 240x134 display. Sizes are fractions of the screen
// ([Ctx.X], [Ctx.Y], [Ctx.Size]), so a slide looks the same at any terminal
// size.
//
// # Frame purity
//
// A frame depends only on its Ctx: no clocks, no math/rand. Any moment can
// then be replayed, snapshotted, rendered to video and pinned by golden
// tests. Use [Hash01] for repeatable noise and Ctx.T, Ctx.Step and Ctx.StepT
// for time.
//
// # Main types
//
//   - [Deck], [Slide]: the talk, and one slide with its steps, notes,
//     [Source] citations and [Transition].
//   - [Ctx]: what changes between frames (T, Step, StepT), the slide's position
//     (Index, Count, Section), plus layout helpers.
//   - [Rect]: a box on the canvas; layout cuts and splits rects
//     ([Rect.CutTop], [Rect.Cols], [Rect.Grid]).
//   - [Scale]: data values mapped onto pixels, with round ticks from
//     [NiceScale], for axes a slide draws itself.
//   - [Theme]: colors, typefaces, chart series colors and an optional overlay
//     the engine uses. Display, Body and Mono fonts are required.
//   - [Scene], [Pixels]: what a View draws on (the engine makes the scene and
//     draws the theme's overlay on it), and its pixel canvas. Scene also has
//     a native character layer; that layer is omitted from video exports.
//     [Scene.Place] hands the engine a keyed element to draw instead, which
//     [TransitionMorph] can move from one slide to the next.
//   - [Text], [Font]: smooth antialiased type, fitted and wrapped to a box,
//     and measured before drawing ([Text.Measure]).
//   - [Rich], [Span]: the same type with mixed fonts, colors and decorations
//     inside a line ([ParseSpans] reads a light markup).
//   - [Block], [FigletFont]: FIGlet block letters scaled to fill a box ([FitBlock]);
//     choose bundled faces with [StockFigletFont] and [StockFigletFontNames], or
//     supply fonts with [LoadFigletFont] and [ParseFigletFont]. Smooth type also
//     accepts consumer fonts through [LoadFont] and [ParseFont].
//   - [GlyphEffect], [BlockEffect]: per-letter and per-cell animations,
//     combined with [Chain] and [BlockChain].
//   - [Table], [TableReveal]: a grid of text fitted to a [Rect] at one size,
//     built up by row or column over steps.
//   - [Composite]: a group drawn on a layer and faded, moved, scaled and
//     clipped as one; [FadeIn], [FlyIn], [Pop], [WipeOut], [Shake] and the
//     rest are its animations, built into a frame by [AppearAt].
//   - [Transition], [Direction]: how a slide enters (push, cover, uncover,
//     wipe, split, fade, fade-through, dissolve, iris, zoom, pixelate, glitch,
//     morph, none), for how long ([Transition.Over]) and from which side
//     ([Transition.From]).
//   - [Connector]: a line, elbow or curve joining two [Rect]s, with arrowheads
//     that sit on the edges and a [Connector.Prog] that draws it on.
//   - [Issue]: a problem [Deck.Review] finds in a build: Code lines clipped,
//     Table rows dropped, text off the canvas or too small to read. Stock
//     components report what they can't fit; a slide's own drawing reports
//     through [Ctx.Fits]. The -review flag prints them.
//
// Shapes beyond the basics ([Pixels.Polygon], [Pixels.Ellipse],
// [Pixels.Polyline], [Pixels.Bezier], [Pixels.DashedLine]) are in shapes.go.
// Stock components ([Panel], [Arrow], [CycleDiagram], [BulletList],
// [PageNumber], [ProgressBar], [Timeline], [Process], [Table], and [Code] with
// its [SyntaxColors], [LineRange] focus, [CodeOverflow] modes and
// [Code.Measure]) and easing helpers ([Ease],
// [CubicBezier], [Spring]) are in draw.go, diagram.go, table.go, code.go and
// anim.go. Charts ([BarChart], [LineChart], [DonutChart], [Sparkline]) and the
// counting [Stat] are in chart.go; their series colors are [Theme.Series]. The
// guide covers the command line, keys and presenter view; the README starts
// with a runnable talk, and the [authoring guide] covers writing one, by
// hand or with an agent.
//
// [hello example]: https://github.com/austinlparker/decker/blob/main/examples/hello/main.go
// [guide]: https://github.com/austinlparker/decker/blob/main/docs/guide.md
// [CLI reference]: https://github.com/austinlparker/decker/blob/main/docs/cli.md
// [authoring guide]: https://github.com/austinlparker/decker/blob/main/docs/authoring.md
package decker

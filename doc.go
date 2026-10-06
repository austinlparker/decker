// Package decker is a terminal slide-deck engine: a talk is a Go program, and
// every slide is a function that draws each frame onto a pixel canvas.
//
// A [Deck] holds a [Theme] and a list of slides. Each [Slide] has a View, a
// function that draws one frame from a [Ctx] onto a [Scene], and [Main] gives
// the deck its command line: present, presenter view, dev reload, snapshots,
// contact sheets and video.
//
//	func main() { decker.Main(talk()) }
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
//   - [Deck], [Slide]: the talk, and one slide with its steps, notes and
//     [Transition].
//   - [Ctx]: what changes between frames (T, Step, StepT), the slide's position
//     (Index, Count, Section), plus layout helpers.
//   - [Rect]: a box on the canvas; layout cuts and splits rects
//     ([Rect.CutTop], [Rect.Cols], [Rect.Grid]).
//   - [Theme]: colors, typefaces and an optional overlay the engine uses.
//   - [Scene], [Pixels]: what a View draws on (the engine makes the scene and
//     draws the theme's overlay on it), and the cells above the pixels.
//     [Scene.Place] hands the engine a keyed element to draw instead, which
//     [TransitionMorph] can move from one slide to the next.
//   - [Text], [Font]: smooth antialiased type, fitted and wrapped to a box.
//   - [Rich], [Span]: the same type with mixed fonts, colors and decorations
//     inside a line ([ParseSpans] reads a light markup).
//   - [Block], [FigFont]: FIGlet block letters scaled to fill a box ([FitBlock]).
//   - [GlyphEffect], [BlockEffect]: per-letter and per-cell animations,
//     combined with [Chain] and [BlockChain].
//   - [Composite]: a group drawn on a layer and faded, moved, scaled and
//     clipped as one; [FadeIn], [FlyIn], [Pop], [WipeOut], [Shake] and the
//     rest are its animations, built into a frame by [AppearAt].
//   - [Transition], [Direction]: how a slide enters (push, cover, uncover,
//     wipe, split, fade, fade-through, dissolve, iris, zoom, pixelate, glitch,
//     morph, none), for how long ([Transition.Over]) and from which side
//     ([Transition.From]).
//
// Stock components ([Panel], [Arrow], [CycleDiagram], [BulletList],
// [PageNumber], [ProgressBar], and [Code] with its [SyntaxColors] and
// [LineRange] focus) and easing helpers ([Ease], [CubicBezier], [Spring]) are
// in draw.go, code.go and anim.go. The package README is the full guide, with
// the command line, keys and presenter view.
package decker

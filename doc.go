// Package decker is a terminal slide-deck engine: a talk is a Go program, and
// every slide is a function that draws each frame onto a pixel canvas.
//
// A [Deck] holds a [Theme] and a list of slides. Each [Slide] has a View, a
// function from [Ctx] to a string of terminal cells, and [Main] gives the
// deck its command line: present, presenter view, dev reload, snapshots,
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
//   - [Ctx]: what changes between frames (T, Step, StepT), plus layout helpers.
//   - [Theme]: colors, typefaces and an optional overlay the engine uses.
//   - [Scene], [Pixels]: the canvas a View draws on, and the cells above it.
//   - [Text], [Font]: smooth antialiased type, fitted and wrapped to a box.
//   - [Block], [FigFont]: FIGlet block letters scaled to fill a box ([FitBlock]).
//   - [GlyphEffect], [BlockEffect]: per-letter and per-cell animations,
//     combined with [Chain] and [BlockChain].
//   - [Transition]: how a slide enters (push, dissolve, wipe, none).
//
// Stock components ([Panel], [Arrow], [CycleDiagram], [BulletList]) and
// easing helpers ([Ease], [Spring]) are in draw.go and anim.go. The package
// README is the full guide, with the command line, keys and presenter view.
package decker

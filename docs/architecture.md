# Where things live

Changing the engine itself? Read [AGENTS.md](../AGENTS.md) first: the
invariants, the golden tests, and a recipe for each kind of addition.

| File | What's in it |
| --- | --- |
| `deck.go`, `slide.go`, `ctx.go` | `Deck`, `Slide` with its `Source` citations, and `Ctx` with its layout in screen fractions |
| `layout.go` | `Rect`: boxes cut from the canvas for layout |
| `cli.go` | `Main`: the command line (live, dev, presenter, list and its JSON outline, handout, snapshot, sheet, video) |
| `handout.go` | `-handout`: a Markdown handout with each slide's thumbnail, notes and sources, and every source once |
| `theme.go`, `color.go` | `Theme`; `RGB`, `Hex`, `Mix` |
| `draw.go` | stock components: `Panel`, `Arrow`, `Label`, `PageNumber`, `ProgressBar`, `Chip`, `CycleDiagram`, `BulletList`… |
| `code.go` | the `Code` component: chroma lexing (cached per source and language), the token-to-color palette (`SyntaxColors`), focus ranges per step, diffs |
| `chart.go` | `BarChart`, `LineChart`, `DonutChart`, `Sparkline`, `Stat`, and `Theme.SeriesColor` |
| `connector.go`, `diagram.go` | `Connector` (lines, elbows and curves between rects) and the `Timeline` and `Process` diagrams |
| `table.go` | `Table`: a grid of text fitted to a rect at one size, with row and column reveals and a walking highlight |
| `font.go`, `fit.go`, `text.go`, `coverage.go`, `memo.go`, `fonts/` | smooth type: font loading and glyphs, fitting and wrapping, drawing with glow and gradients, coverage masks, cached fits |
| `rich.go`, `richmarkup.go` | `Rich` text: spans with their own font, color and decoration laid out and drawn on the `Text` machinery, fitted to a box; `ParseSpans` markup |
| `figlet.go`, `block.go`, `blockfit.go`, `blockglyph.go`, `fonts/figlet/` | block letters: bundled font catalog (`StockFigletFont`, `StockFigletFontNames`), custom font loading (`LoadFigletFont`, `ParseFigletFont`), drawing, fitting and block-character glyphs; hash-pinned BDF and outline conversion scripts |
| `effects.go`, `blockfx.go` | letter animations: `GlyphEffect` for `Text`, `BlockEffect` for `Block` |
| `pixels.go`, `shapes.go`, `pixelart.go`, `image.go` | the pixel canvas and its basic shapes; polygons, ellipses, polylines, curves, dashes and gradients; pixel art; images |
| `anim.go` | easing (`Ease*`, `CubicBezier`), springs, noise |
| `layer.go`, `animate.go` | `Composite`: draw a group on a layer and fade, move, scale and clip it; the element animations (`FadeIn`, `FlyIn`, `Pop`, `WipeOut`, `Shake`, `AppearAt`...) built on it |
| `scene.go`, `grid.go`, `pool.go` | combine the pixel canvas and character layer into terminal cells; reused frame buffers |
| `render.go` | a slide's frame: the scene `View` draws on, then placed elements and the overlay, panics caught |
| `review.go`, `reviewlog.go`, `reviewreport.go`, `reviewimage.go` | `Deck.Review`: every build checked at several sizes while it draws, with what components report (`Ctx.Fits`, `Ctx.Report`, text off the canvas or too small), which element inked each pixel (overlaps, the overlay drawing over the slide), and frames compared (builds that add nothing, impure slides, morphs with nothing to move); the `-review` report as text, Markdown, JSON and annotated images |
| `transition.go`, `transfx.go`, `morph.go`, `direction.go` | slide transitions: each mixes two scenes, for the terminal and video alike. `transition.go` has the kinds and directions and the ones that move or sweep frames (push, cover, uncover, split, wipe, dissolve); `transfx.go` the ones that rework pixels (fades, iris, zoom, pixelate, glitch); the morph moves placed elements |
| `model.go`, `keys.go`, `termout.go`, `dev.go` | the app: navigation, the key table, writing frames, dev reload |
| `presenter.go`, `link.go`, `preview.go` | the presenter view, the socket link to the deck, slide previews |
| `present_window.go`, `kitty.go` | launch a Ghostty deck window with its own font size; upload and manage full-resolution presenter images |
| `png.go` | PNG snapshots and contact sheets, painted from cell grids |
| `video.go` | rendering a deck to a video |
| `decktest/` | the test suite for decks |

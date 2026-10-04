# Working on the decker engine

The Go library `github.com/austinlparker/decker`: a terminal slide-deck engine.
A talk is a separate `main` package, usually in another repo, that builds a
`Deck` and calls `Main`. Read `doc.go` for the model and `README.md` ("Where
things live") for the file map.

## Invariants

- **Frame purity.** A frame depends only on its `Ctx`. No `time.Now`, no
  `math/rand`, no globals that change between frames; use `Hash01` for noise.
  Anything stateful breaks replay, `-snapshot`, video and the goldens.
- **The engine owns the scene.** A slide's `View(c Ctx, sc *Scene)` only
  draws; `renderSlide` makes the scene, runs `View`, draws `Theme.Overlay`
  once and catches panics (drawn into the pixels). Cells (`renderSlideGrid`:
  live, snapshots, transitions, previews) come from `Scene.toGrid`; video
  reads `sc.Px` directly and has no character layer. Styled strings are
  parsed only for the engine's own chrome (footer, panels, help box).
- **Goldens pin every byte.** `gallery_test.go` (`testdata/gallery.golden`) hashes
  every pixel and cell of a deck that exercises each exported drawing API.
  `golden_internal_test.go` (`testdata/internal.golden`) hashes what the gallery
  can't reach: cell and video transitions, PNG export, the live model's screen,
  the terminal writer's escape sequences, the presenter view, kitty previews.
- **Keep float operation order.** Reordering or fusing arithmetic (`a*b+c`,
  summing in a different order, changing a constant's type) moves pixels and
  fails goldens. A refactor must leave every hash unchanged.
- **Never re-record goldens to make a test pass.** Re-record only when the
  output is meant to change, and say in the commit which keys changed and why.
  `UPDATE_GOLDEN=1 go test ./...` rewrites both files; review `git diff testdata`
  and confirm only the intended keys moved. A pure refactor has an empty diff.
- **Perf budget.** Per-frame paths are hot: a 682x171 frame is ~117k cells and
  ~2.8MB of pixels, made 60 times a second. Don't allocate per frame; reuse
  buffers through `pool.go` and cache pure results with `memo.go`. Measure with
  `go test -run '^$' -bench Live` before and after touching `scene.go`, `grid.go`,
  `pixels.go`, `text.go`, `termout.go`, `model.go` or `transition.go`.
- **The exported API is used by talks in other repos.** Don't rename, remove or
  change the behavior of an exported identifier without being asked. Additions
  are fine. Unexported code is free to change if the goldens hold.

## Commands

```sh
go test ./...                       # everything, including goldens (a few seconds)
go test -short ./...                # skips the slow golden and kitty tests
go vet ./...
gofmt -l .                          # must print nothing
go test -run '^$' -bench Live       # the engine's share of a live frame
UPDATE_GOLDEN=1 go test ./...       # re-record goldens: only for intended output changes
```

## Comments

Every exported identifier has a godoc comment that starts with its name. Other
comments say *why*: a constraint, an invariant, a surprise. No narration of what
the next line does, no history ("now we..."), no commented-out code. Keep the
`doc.go` type map and the README tables in step with any file or type you add.

## Recipes

`gallery_test.go` is a deck whose slides use every exported drawing API, with
frame-pure `View`s. Each recipe ends the same way: exercise the addition there,
run `go test ./...`, and re-record only the new keys
(`UPDATE_GOLDEN=1 go test -run TestGalleryGolden`).

**Add a transition** (`transition.go`). Append a constant to the `Transition`
block (at the end, so existing values keep their numbers). Write a `xxxCells`
func (`out, from, to *grid, p float64, forward bool, t *Theme`, ending each row
with `fixWideEdges`) and a `xxxPixels` func (`from, to []byte, w, h int, p
float64, t *Theme`, mixing into `to` in place). Register both in the
`transitions` map; a kind with no entry cuts straight to the new frame. Add the
kind to `transitionNames` and `transitionKinds` in `golden_internal_test.go`,
and use it on a slide in `gallery_test.go`. Mention it in `Slide.Transition` docs
and the README. Easing is the implementation's job: `p` is linear.

**Add a letter effect** (`effects.go`). Write `func Name(t, ...) GlyphEffect`
returning `func(i int) GlyphFX`. `t` is seconds since the effect starts; the zero
`GlyphFX` hides the glyph, `GlyphFX{Alpha: 1}` leaves it alone. Use `staggered`
for per-letter delays and `Hash01` for noise. It composes through `Chain` with no
more work. Add it to the "Letter effects" slide in `gallery_test.go` and to the
README Toolbox row.

**Add a block effect** (`blockfx.go`). Write `func BlockName(t, ...) BlockEffect`
returning `func(c BlockCell) BlockFX`; same rules as above, with `BlockCell`
giving `Col`, `Row`, `U`, `Char`, `H` and `BlockFX` adding `Bright` and `Color`.
It composes through `BlockChain`. Add it to the "Block effects" slides in
`gallery_test.go` and to the README Toolbox row.

**Add a stock component** (`draw.go`). Follow the convention in `Panel`'s doc:
take `(c Ctx, p *Pixels, ...)` with pixel coordinates, size from `c.Unit` and
`c.SmallText`, take colors and fonts from `c.Theme`, reveal builds with
`c.Reached` and `c.Since`, return the size drawn. Many options means a struct
with a `Draw` method, like `CycleDiagram`. Draw into `p`, not into a new
`Scene`. Add it to the "Components" slide in `gallery_test.go` and to the README
Toolbox.

**Add a key binding** (`keys.go`, `model.go`). Add one row to `bindings`: the
space-separated key names (as `tea.KeyPressMsg.String` reports them), an action
name, `nav` true if the presenter view may press it on the deck's behalf, and
the help-box text (empty to hide it). Handle the action name in the `switch` in
`model.handleKey`. Presenter-only keys (`t`, `T`) are handled in
`presenter.handleKey` instead. Update the README Keys table. The help box
is generated from `bindings`.

**Add a CLI flag or mode** (`cli.go`). Add a field to `options`, register it in
`parseFlags` with `flag.*Var` (the usage string starts "with -mode:" if it only
applies to one). For a mode, add a `case` to the `switch` in `run`, ordered
before the cases it should win over, and write `runXxx(d *Deck, o options)
error`. Rendering without a terminal should go through `stillFrame` or
`renderSlideGrid`/`Deck.Render` (cells), or `renderSlide` (pixels), not a live
model. Update "Running a deck" in the
README.

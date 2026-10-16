# Working on the decker engine

The Go library `github.com/austinlparker/decker`: a terminal slide-deck engine.
A talk is a separate `main` package, usually in another repo, that builds a
`Deck` and calls `Main`. Read `doc.go` for the model, `README.md` for the
quick start, and `docs/architecture.md` for the file map. Writing a talk
rather than the engine? Read `docs/authoring.md` and start from
`examples/starter`, whose `AGENTS.md` goes into the talk's repo.

## Invariants

- **Frame purity.** A frame depends only on its `Ctx`. No `time.Now`, no
  `math/rand`, no globals that change between frames; use `Hash01` for noise.
  Anything stateful breaks replay, snapshots, video and the references;
  `decktest.Slides` draws each build twice and fails if they differ.
- **The engine owns the scene.** A slide's `View(c Ctx, sc *Scene)` only
  draws; `drawSlide` makes the scene and runs `View`, and `finish` draws the
  elements it placed and `Theme.Overlay`, once; both catch panics (drawn into
  the pixels). Transitions mix two scenes from `drawSlide`: most finish both
  first, and the morph moves the elements before drawing them. Cells (live, snapshots, previews) come from
  `Scene.toGrid` after that; video reads `sc.Px` directly and has no
  character layer. Styled strings are
  parsed only for the engine's own chrome (footer, panels, help box).
- **References catch what anyone could see; text is exact.** `gallery_test.go`
  keeps an image of every slide of a deck that exercises each exported
  drawing API (`testdata/gallery`), and `testdata/gallery-review.golden` lists
  what `Deck.Review` finds in it. `golden_internal_test.go` keeps what the
  gallery can't reach (`testdata/internal`): images of transitions in cells
  and video, the video loop, PNG export, the live model's and the presenter's
  screens; and, as text, their character layers, the terminal writer's escape
  sequences, and traces through Update. Images are compared halved and
  within a tolerance (`internal/visual`), because the same source doesn't
  render the same bits everywhere: Go fuses `a*b+c` into one instruction on
  arm64 (and amd64 with `GOAMD64=v3`), which moves the odd pixel by
  rounding. So arithmetic may be reordered; output a person could tell
  apart may not change. Text and frames from whole-number inputs are exact.
- **Never re-record references to make a test pass.** Re-record only when
  the output is meant to change, and say in the commit which ones changed
  and why. `UPDATE_GOLDEN=1 go test ./...` rewrites only the references that
  no longer match; a failure leaves what it drew and a diff image in the
  directory it names. Review `git diff testdata` (the images too) and confirm
  only the intended ones moved. A pure refactor has an empty diff.
- **Perf budget.** Per-frame paths are hot: a 682x171 frame is ~117k cells and
  ~2.8MB of pixels, made 60 times a second. Don't allocate per frame; reuse
  buffers through `pool.go` and cache pure results with `memo.go`. Measure with
  `go test -run '^$' -bench Live` before and after touching `scene.go`, `grid.go`,
  `pixels.go`, `text.go`, `termout.go`, `model.go` or `transition.go`.
- **API changes: change in place, never leave a shim.** Decker is pre-1.0.
  When an exported name, signature or behavior should change, change it, move
  every caller (gallery, examples, README, docs), and say in the PR that it
  breaks. Talks in other repos import the API, so make each break worth its
  edit, and make it once rather than in steps. Don't keep the old form
  compiling: no alias, no `Deprecated:` wrapper, no function that forwards
  its arguments to the new one, no `FooV2` beside `Foo`. `TestNoCompatShims`
  (`shim_test.go`) type-checks the API and fails on aliases, `Deprecated:`
  declarations, vars and consts that rename another, and forwarders; a
  `FooV2` with its own body gets past it and is still a shim. Its
  grandfathered list only shrinks. Unexported code is free to change if the
  references hold.
- **Merging releases.** A merge to main that changes library code is tagged
  and released by CI, the version bumped from the API diff (`apidiff`): a
  break bumps the major (minor at v0), an addition the minor. Say in the PR
  when it changes the exported API.

## Commands

```sh
go test ./...                       # the engine, including references (a few seconds)
(cd bsky && go test ./...)          # bsky, a module of its own: ./... above leaves it out
(cd examples/showcase && go test ./...)   # the example talk, a module too; posts a clip, so needs ffmpeg
go test -short ./...                # skips the slower reference tests
go vet ./...
gofmt -l .                          # must print nothing
go test -run '^$' -bench Live       # the engine's share of a live frame
(cd examples/showcase && go run . review /tmp/review)   # what a review reports, on a real deck
UPDATE_GOLDEN=1 go test ./...       # re-record references: only for intended output changes
```

## Comments

Every exported identifier has a godoc comment that starts with its name. Other
comments say *why*: a constraint, an invariant, a surprise. No narration of what
the next line does, no history ("now we..."), no commented-out code. Keep the
`doc.go` type map and the guide and architecture tables in step with any
file or type you add.

## Recipes

`gallery_test.go` is a deck whose slides use every exported drawing API, with
frame-pure `View`s. Each recipe ends the same way: exercise the addition there,
run `go test ./...`, record the new references
(`UPDATE_GOLDEN=1 go test -run TestGalleryGolden`), and look at them.

**Add a transition** (`transition.go`). Append a kind to the `transitionKind`
block (at the end, so existing kinds keep the numbers the references
record) and an exported `Transition` var for it; if its default time isn't
`TransitionDuration`, add a case to `Transition.Duration`. Write one
`transitionFunc` (`from, to *Scene, p float64, forward bool, t *Theme`) that
mixes `from` into `to` in place: the pixels, and the character layer through
`moveChars`. The terminal, snapshots and video all use it. Size bands and
blocks from the frame's width, not in fixed cells, so it looks the same at
any resolution. Register it in the `transitions` map, keyed by kind and wrapped in `finished`,
which draws both slides' placed elements first; only a transition that moves
elements itself, like `morph`, goes in unwrapped. A kind with no entry cuts
straight to the new frame. Add the
kind to `transitionNames` and `transitionKinds` in `golden_internal_test.go`,
and use it on a slide in `gallery_test.go`. Mention it in `Slide.Transition` docs
and the guide. Easing is the implementation's job: `p` is linear.

**Add a letter effect** (`effects.go`). Write `func Name(t, ...) GlyphEffect`
returning `func(i int) GlyphFX`. `t` is seconds since the effect starts; the zero
`GlyphFX` hides the glyph, `GlyphFX{Alpha: 1}` leaves it alone. Use `staggered`
for per-letter delays and `Hash01` for noise. It composes through `Chain` with no
more work. Add it to the "Letter effects" slide in `gallery_test.go` and to the
guide Toolbox row.

**Add a block effect** (`blockfx.go`). Write `func BlockName(t, ...) BlockEffect`
returning `func(c BlockCell) BlockFX`; same rules as above, with `BlockCell`
giving `Col`, `Row`, `U`, `Char`, `H` and `BlockFX` adding `Bright` and `Color`.
It composes through `BlockChain`. Add it to the "Block effects" slides in
`gallery_test.go` and to the guide Toolbox row.

**Add a stock component** (`draw.go`). Follow the convention in `Panel`'s doc:
take `(c Ctx, p *Pixels, ...)` with pixel coordinates, size from `c.Unit` and
`c.SmallText`, take colors and fonts from `c.Theme`, reveal builds with
`c.Reached` and `c.Since`, return the size drawn. Many options means a struct
with a `Draw` method, like `CycleDiagram`. Draw into `p`, not into a new
`Scene`. Start `Draw` with `defer c.within("Name")()` so the text it draws
reports to a review as part of it, and report what it can't fit: `c.Fits`
for a block bigger than its rect, `c.review.add` (behind `c.review != nil`)
for anything with its own code, like `Code`'s "code-clipped". Never clip or
drop content silently. Add it to the "Components" slide in `gallery_test.go`
and to the guide Toolbox, and check `testdata/gallery-review.golden`.

**Add a key binding** (`keys.go`, `model.go`). Add one row to `bindings`: the
space-separated key names (as `tea.KeyPressMsg.String` reports them), an action
name, `nav` true if the presenter view may press it on the deck's behalf, and
the help-box text (empty to hide it). Handle the action name in the `switch` in
`model.handleKey`. Presenter-only keys (`t`, `T`) are handled in
`presenter.handleKey` instead. Update the guide Keys table. The help box
is generated from `bindings`.

**Add a CLI flag or command** (`cli.go`). The command line is parsed by
kong. A command is a field of `cli` tagged `cmd:""` with a `help:` line; its
type holds its flags, tagged with `default:`, `help:`, `enum:` for a fixed set
or `arg:""` for a positional, and has `Run(d *Deck) error`, plus
`Validate() error` for checks the tags can't say. Commands share flags by
embedding a group (`position`, `deckFlags`, `frameFlags`). Put a flag on the
commands it applies to and no others, so kong rejects it elsewhere. Rendering
without a terminal should go through `Deck.still` or `Deck.cells` (cells),
or `renderSlide` (pixels), not a live model. Add cases to `TestCommandLine` and `TestCommandLineRejects`, and
update "Running a deck" in `docs/guide.md` and `docs/cli.md`.

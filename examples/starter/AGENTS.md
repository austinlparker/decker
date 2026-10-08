# Writing this talk

This talk is a Go program built on [decker](https://github.com/austinlparker/decker):
every slide is a function that draws a frame from a `decker.Ctx`. Read the
decker [authoring guide](https://github.com/austinlparker/decker/blob/main/docs/authoring.md)
once; this file is the short version.

## Build the explanation; don't rebuild the basics

- **Use the stock components for routine content:** `Code` (highlighting,
  `Focus`, `Diff`, `Overflow`, `Excerpt`), `Table`, `BarChart`/`LineChart`/
  `DonutChart`/`Stat`, `Connector`, `Font.Fit`/`Text`/`Rich`, `FitBlock`/
  `Block`, the element animations and the transitions. Don't write your own.
- **Build the visual that makes the point yourself**, for this talk, from
  primitives: `Rect` cuts, `Text.Measure`, `NiceScale`, `Pixels` shapes,
  `c.Reached`/`c.Since` and easing. A canned diagram won't build up in the
  order you'll say it. Copy from decker's `examples/recipes` (a trace
  waterfall) and change it.
- **One idea per slide.** The headline says the point. Detail goes in
  `Notes`, citations in `Sources`.

## How to write a slide

- **Lay out from rects:** `c.Frame()`, `Inset`, `CutTop`/`CutLeft`, `Rows`,
  `Cols`. Never use pixel constants. `CutTop`, `CutBottom`, `CutLeft` and
  `CutRight` return the strip first and the rest second.
- **Measure, then place:** every `Draw` returns its size, and
  `Text.Measure`, `Code.Measure` and `Block.Size` measure without drawing.
- **Frames are pure:** the same `Ctx` gives the same frame. No `time.Now`,
  no `math/rand` (use `decker.Hash01`), no variables changed between frames.
- **Builds:** `Steps` is how many there are. `c.Reached(n)` says whether
  build `n` (0-based) is showing, and `c.Since(n)` how long ago it began.
- **Text size:** keep text at `c.SmallText(font)` or bigger.
- **Your own layout:** if it might not fit, choose the fallback with a plain
  size comparison, then call `c.Fits(name, rect, w, h)` on the layout you
  draw, so the review hears if even that doesn't fit.

## Check every change

```sh
go run . review review       # every build at 240x67, 320x90 and 682x171
go test ./...                # decktest.Slides and decktest.Review
```

- **Fix every error and warning** the review prints. It names the slide,
  build, size and element, and how much is wrong.
- **Look at the images,** not just the list: `review/index.md` links an
  annotated frame for each build with an issue, and `review/sheet-*.png`
  shows every build at once. The last build can look fine while an earlier
  one is broken.
- **If an issue is intended,** list its code in the slide's `Allow` and say
  why in a comment.
- **Before you're done:** `go run . handout handout` writes the notes and
  sources with a thumbnail per slide, to rehearse from.

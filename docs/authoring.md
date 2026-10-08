# Authoring a talk

[README](../README.md) · [Guide](guide.md) · [CLI reference](cli.md)

This page is for whoever writes the slides, a person or an agent: what decker
gives you, what it expects you to build, and the loop that tells you when a
slide is wrong. Start a talk from the [starter](../examples/starter), whose
[AGENTS.md](../examples/starter/AGENTS.md) carries the short version of this
page into your repository.

## What to build, and what not to

Decker is a canvas, primitives and a feedback loop, not a template library.
Some things are the same in every talk, and you shouldn't build them
yourself:

| Don't build | Use |
| --- | --- |
| Syntax highlighting, line focus, diffs | `Code` |
| Tables that fit their box | `Table` |
| Bar, line and donut charts, counting stats | `BarChart`, `LineChart`, `DonutChart`, `Stat` |
| Text fitted and wrapped to a box | `Text.Fit`, `Rich.Fit` |
| Block-letter titles | `FitBlock`, `Block` |
| Lines and arrows between boxes | `Connector` |
| Fades, flies, pops and wipes | `AppearAt` and the element animations |
| Transitions and magic move | `Slide.Transition`, `Scene.Place` |
| Navigation, presenter view, export | `Main` |

The visual that carries your explanation is different: the trace that shows
where the time went, the architecture that shows why the queue matters, the
request that shows which hop failed. Build it for this talk, from
primitives, so it shows exactly the point you're making and builds up in the
order you'll say it. A generic "diagram component" can't know either.

The primitives for that are layout (`Rect` cuts, `Rows`, `Cols`, `Grid`,
`Anchor`), measurement (`Text.Measure`, `Text.Fit`,
`Code.Measure`, the sizes every component's `Draw` returns), scales
(`NiceScale` for an axis of your own), shapes (`Pixels.Rect`, `RoundRect`,
`Polygon`, `Bezier`, `Line`, gradients), and time (`c.Reached`, `c.Since`,
`Ease`, `Spring`). The [recipes](../examples/recipes) are worked examples,
starting with a trace waterfall, to copy and change, not to import.

## How a slide is put together

1. **Lay out from rects, not pixels.** Start from `c.Frame()` and cut:
   `head, body := c.Frame().Inset(c.Unit(0.04), c.Unit(0.03)).CutTop(c.Y(0.18))`.
   Fractions of the screen look the same at every terminal size; pixel
   numbers don't.
2. **Measure before you draw.** Every component's `Draw` returns the size it
   drew; lay out the next thing below it (`y += h + gap`). To place
   something before drawing it, measure it: `Text.Measure`, `Rich.Measure`,
   `Code.Measure`, `Block.Size`.
3. **Compute your layout as a function of the rect and the data, then
   draw.** Keep it pure: the same `Ctx` must give the same frame, with no
   clocks, no `math/rand` (use `Hash01`) and no state kept between frames.
4. **Degrade on purpose, and say when even that doesn't fit.** If your
   layout needs more room than it has, choose what to give up (fewer
   labels, a shorter excerpt) rather than letting it clip, then check the
   layout you kept with `c.Fits`, which tells the review if it still
   doesn't fit:

   ```go
   lay := layoutWaterfall(spans, r)
   if lay.W > r.W || lay.H > r.H {
       lay = layoutWaterfall(spans[:8], r)
   }
   c.Fits("waterfall", r, lay.W, lay.H)
   ```

5. **Reveal in the order you'll talk.** `Steps` is the number of builds;
   `c.Reached(n)` says whether build `n` (0-based) is showing and
   `c.Since(n)` how long ago it started, for easing it in. Count builds
   from the data when the visual is data (`Steps: len(spans)`).
6. **Keep text readable.** Nothing below `c.SmallText(font)`. A slide holds
   one idea; the rest goes in `Notes`, and citations in `Sources`.

## The loop

Write a slide, then look at it the way an audience will: every build, at
every size.

```sh
go run . -review review
```

That checks every build of every slide, settled, at 240×67, 320×90 and
682×171 cells, and prints what's wrong with slide, build, size, element and
numbers:

```
11.2  Collector config  320x90  error  code-clipped  Code "otel.yaml": 5 of 10 lines visible (needs 236px tall, has 118px) at 12px, the smallest readable size
```

Then open `review/index.md`. It lists the same issues by slide, with an
image of each build that has one: the frame with what it drew outlined and
each issue boxed and numbered. `review/sheet-320x90.png` shows every build
at a glance, framed red or amber where something's wrong, so a broken
intermediate build stands out. Look at the images, not just the list.

| Severity | Means | Do |
| --- | --- | --- |
| error | content is lost: clipped, dropped, off the canvas, a panic | fix it; `-review` exits 1 |
| warning | it shows, but badly: text too small, two things on top of each other | fix it, or `Allow` it on the slide if it's meant |

A slide that means to have an issue says so with
`Allow: []string{"text-offcanvas"}`, which keeps the report about what's
actually wrong.

Put the review in the talk's tests so it keeps holding:

```go
func TestSlides(t *testing.T) { decktest.Slides(t, talk()) }
func TestReview(t *testing.T) { decktest.Review(t, talk()) }
```

## Notes, sources and the handout

`Slide.Notes` are what you'll say; the presenter view shows them.
`Slide.Sources` are what you cite, kept on the slide with what cites
them. `go run . -handout handout` writes `handout/handout.md`:
every slide's thumbnail, notes and sources, then every source once with the
slides that cite it. Rehearse from it, or give it to the audience.

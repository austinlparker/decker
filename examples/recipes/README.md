# Recipes

Worked examples of the visuals that carry a talk's explanation, built from
decker's primitives. Stock components cover routine content (code, tables,
charts). A trace waterfall is better purpose-built
for the point you're making, so these recipes are code to copy and adapt,
not an API to call.

```sh
go run ./examples/recipes                    # present it
go run ./examples/recipes review review      # check every build at three sizes
```

| File | Recipe |
| --- | --- |
| [`waterfall.go`](waterfall.go) | A trace waterfall: spans indented under their parents, names in a column sized with `Text.Measure`, bars on a `NiceScale` millisecond axis colored by service, duration labels inside the bar when they fit, else after it (or before it, near the right edge). One span per build, then the critical path highlighted. |

[`style.go`](style.go) has the theme and the `heading` template the recipes
draw their titles with; use your talk's own in its place.

## How a recipe is built

Each one follows the same steps, and the comment at the top of each file
says how it applies them:

1. **Lay out first.** A pure function of the rect and the data measures the
   text, builds the scales and cuts the rects, and says how much room it
   needs. It draws nothing.
2. **Check, and degrade on purpose.** The slide hands that size to
   `c.Fits`. When it doesn't fit, the recipe falls back to a layout that
   gives something up deliberately (labels, the legend, the tail of the
   data) rather than shrinking text below `c.SmallText` or
   letting it clip. `c.Fits` checks the layout it keeps, so the review
   hears if even that doesn't fit.
3. **Draw** from the layout, frame-pure: no clocks, no `math/rand`
   (`decker.Hash01` for noise), nothing kept between frames.
4. **Reveal** over the slide's builds with `c.Reached`, `c.Since` and
   easing, with `Slide.Steps` following the data.

## Using one

1. Copy the recipe's file into your talk's `main` package and add its slide
   to your deck.
2. Change the data and the knobs listed in its header comment. Swap
   `heading` for your title template.
3. Run `go run . review review` and read `review/index.md` and the sheets,
   until it reports no issues at 240x67, 320x90 and 682x171.

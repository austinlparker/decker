// Command recipes is a deck of worked recipes for the visuals that carry a
// talk's explanation, starting with a trace waterfall. The engine has stock
// components for routine content (code, tables, charts); visuals like these
// are better purpose-built from primitives, so each recipe here is a small
// function to copy into a talk and edit, not an API to call.
//
// Every recipe follows the same pattern: lay out first, as a pure function of
// a rect and the data; check the layout against the rect with c.Fits, and
// degrade on purpose when it doesn't fit; then draw, revealing over the
// slide's builds. Run the review to see it hold at every size:
//
//	go run ./examples/recipes review review
package main

import "github.com/austinlparker/decker"

func main() { decker.Main(talk()) }

func talk() decker.Deck {
	return decker.Deck{Name: "decker-recipes", Theme: theme, Slides: []decker.Slide{
		waterfallSlide(),
	}}
}

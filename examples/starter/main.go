// Command starter is a talk to copy: a theme, a slide template, three slides
// and the tests that keep them right. Copy this directory into a new module,
// change the module path, and replace the slides. AGENTS.md tells an agent
// writing the talk how decker expects it to be built and checked.
package main

import "github.com/austinlparker/decker"

func main() { decker.Main(talk()) }

func talk() decker.Deck {
	return decker.Deck{Name: "starter", Theme: theme, Slides: []decker.Slide{
		titleSlide(),
		pointSlide(),
		budgetSlide(),
	}}
}

package main

import (
	"testing"

	"github.com/austinlparker/decker/decktest"
)

func TestSlides(t *testing.T) { decktest.Slides(t, talk()) }

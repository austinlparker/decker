package decker_test

import (
	"testing"

	"github.com/austinlparker/decker"
	"github.com/austinlparker/decker/decktest"
)

func TestDeckSlides(t *testing.T) { decktest.Slides(t, *decker.SampleDeck()) }

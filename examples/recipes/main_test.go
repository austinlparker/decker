package main

import (
	"testing"

	"github.com/austinlparker/decker/decktest"
)

func TestSlides(t *testing.T)      { decktest.Slides(t, talk()) }
func TestReview(t *testing.T)      { decktest.Review(t, talk()) }
func TestGolden(t *testing.T)      { decktest.Golden(t, talk(), "testdata/golden.txt") }
func BenchmarkFrames(b *testing.B) { decktest.Frames(b, talk()) }

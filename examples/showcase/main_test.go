package main

import (
	"os"
	"slices"
	"testing"

	"github.com/austinlparker/decker/decktest"
)

func TestSlides(t *testing.T) { decktest.Slides(t, talk()) }
func TestReview(t *testing.T) { decktest.Review(t, talk()) }
func TestGolden(t *testing.T) { decktest.Golden(t, talk(), "testdata/golden") }

// TestPosts checks every post against Bluesky's limits, and that each post
// still shows the kind it's there to.
func TestPosts(t *testing.T) {
	posts := decktest.Posts(t, talk())
	var media []int
	for _, p := range posts {
		media = append(media, len(p.Media))
	}
	// The opener, then the slides' posts: two on Numbers, none on Charts,
	// Diagram or Magic move.
	want := []int{0, 1, 2, 1, 0, 1, 2, 6, 1, 1, 1}
	if !slices.Equal(media, want) {
		t.Fatalf("media per post %v, want %v", media, want)
	}
	if clip := posts[3].Media[0]; !clip.Video {
		t.Errorf("Letter effects posts %s, not a clip", clip.Path)
	}
	st, err := os.Stat(posts[8].Media[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Size() <= 2_000_000 {
		t.Errorf("Film grain's PNG is %d bytes, so it posts as a PNG, not the JPEG it's there to show", st.Size())
	}
}

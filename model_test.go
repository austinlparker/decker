package decker

import "testing"

func TestDeckFollowsLinkCommands(t *testing.T) {
	d := testDeck()
	m := newModel(d, 0, 0, 30, nil)
	for _, c := range []struct {
		cmd  linkCmd
		want int // slide index afterwards
	}{
		{linkCmd{Key: "right"}, 1},
		{linkCmd{Slide: 3}, 2},
		{linkCmd{Key: "end"}, len(d.Slides) - 1},
		{linkCmd{Key: "home"}, 0},
		{linkCmd{Key: "bogus"}, 0},
		{linkCmd{Key: "q"}, 0}, // only navigation keys are allowed
	} {
		next, cmd := m.Update(c.cmd)
		m = next.(model)
		if m.idx != c.want || cmd != nil {
			t.Fatalf("%+v: slide %d (want %d), cmd %v", c.cmd, m.idx+1, c.want+1, cmd != nil)
		}
	}
}

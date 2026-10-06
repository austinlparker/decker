package decker

import (
	"math/rand/v2"
	"reflect"
	"testing"
	"time"
)

// Property tests use native Go fuzzing. go test ./... runs regression seeds
// and deterministic generated programs; mutation fuzzing runs one target at
// a time, for example:
//
//	go test -run '^$' -fuzz '^FuzzCharacterOverwrite$' -fuzztime=30s
//
// Go saves minimized failures under testdata/fuzz/<target>. Keep those inputs
// as regressions when fixing failures. Bound generated sizes and sequence
// lengths so ordinary test runs and fuzz workers stay practical.

// navigationPosition is an independent cursor over the deck's build states.
// It deliberately has no clocks, rendering, or transition machinery.
type navigationPosition struct {
	slide, step int
	blank       blankMode
	help, notes bool
}

func (p *navigationPosition) apply(key string, steps []int) {
	switch key {
	case "space", "left", "]", "[", "g", "G", "r", "n", "?", "esc":
		p.blank = blankNone
	}
	switch key {
	case "space":
		if p.step+1 < steps[p.slide] {
			p.step++
		} else if p.slide+1 < len(steps) {
			p.slide++
			p.step = 0
		}
	case "left":
		if p.step > 0 {
			p.step--
		} else if p.slide > 0 {
			p.slide--
			p.step = steps[p.slide] - 1
		}
	case "]":
		if p.slide+1 < len(steps) {
			p.slide++
			p.step = 0
		}
	case "[":
		if p.slide > 0 {
			p.slide--
			p.step = 0
		}
	case "g":
		p.slide, p.step = 0, 0
	case "G":
		p.slide, p.step = len(steps)-1, 0
	case "r":
		p.step = 0
	case "?":
		p.help = !p.help
	case "n":
		p.notes = !p.notes
	case "esc":
		p.help = false
	case "b", "w":
		mode := blankBlack
		if key == "w" {
			mode = blankWhite
		}
		if p.blank == mode {
			p.blank = blankNone
		} else {
			p.blank, p.help, p.notes = mode, false, false
		}
	}
}

var propertyKeys = []string{"space", "left", "]", "[", "g", "G", "r", "b", "w", "?", "n", "esc", "ctrl+l", "unbound"}

func checkNavigation(t *testing.T, m model, want navigationPosition) {
	t.Helper()
	got := navigationPosition{m.idx, m.step, m.blank, m.showHelp, m.showNotes}
	if got != want {
		t.Fatalf("navigation = %+v, want %+v", got, want)
	}
}

func TestNavigationFiniteStates(t *testing.T) {
	// Every position, blank mode, help/notes combination, and navigation
	// action, for all two-slide decks with zero through three declared steps.
	for a := 0; a <= 3; a++ {
		for b := 0; b <= 3; b++ {
			slides := []Slide{{Steps: a, Transition: TransitionNone}, {Steps: b, Transition: TransitionNone}}
			steps := []int{max(a, 1), max(b, 1)}
			for slide, n := range steps {
				for step := range n {
					for blank := blankNone; blank <= blankWhite; blank++ {
						for flags := range 4 {
							for _, key := range propertyKeys {
								m := model{slides: slides, idx: slide, step: step, blank: blank, showHelp: flags&1 != 0, showNotes: flags&2 != 0}
								want := navigationPosition{slide, step, blank, m.showHelp, m.showNotes}
								want.apply(key, steps)
								m, _ = m.handleKey(key)
								checkNavigation(t, m, want)
							}
						}
					}
				}
			}
		}
	}
}

func FuzzNavigationSequences(f *testing.F) {
	f.Add([]byte{0}, []byte{0, 0, 1, 7, 7, 8, 9, 0})
	rng := rand.New(rand.NewPCG(7, 8))
	for range 64 {
		deck, commands := make([]byte, rng.IntN(8)+1), make([]byte, 64)
		for i := range deck {
			deck[i] = byte(rng.Uint32())
		}
		for i := range commands {
			commands[i] = byte(rng.Uint32())
		}
		f.Add(deck, commands)
	}
	f.Fuzz(func(t *testing.T, deck, commands []byte) {
		if len(deck) == 0 {
			deck = []byte{0}
		}
		deck = deck[:min(len(deck), 16)]
		slides, steps := make([]Slide, len(deck)), make([]int, len(deck))
		for i, b := range deck {
			slides[i] = Slide{Steps: int(int8(b) % 8), Transition: TransitionNone}
			steps[i] = max(slides[i].Steps, 1)
		}
		m := model{slides: slides, now: time.Unix(1000, 0)}
		want := navigationPosition{}
		for i, b := range commands[:min(len(commands), 256)] {
			key := propertyKeys[int(b)%len(propertyKeys)]
			want.apply(key, steps)
			m, _ = m.handleKey(key)
			checkNavigation(t, m, want)
			c := m.ctx(5)
			if c.Index != want.slide || c.Count != len(slides) || c.Step != want.step || c.StepT < 0 || c.T < 0 {
				t.Fatalf("command %d left invalid context: %+v", i, c)
			}
			m.now = m.now.Add(time.Second)
		}
	})
}

func propertyFrame(w, h int, old bool) *Scene {
	s := NewScene(w, h, testTheme)
	base, ch := 40, "b"
	if old {
		base, ch = 120, "a"
	}
	for i := range s.Px.Pix {
		s.Px.Pix[i] = RGB{float32((base + i) % 256), float32(i % 256), float32(base)}
	}
	for y := range h {
		s.Text(0, y, ch+"界"+ch, nil)
	}
	s.finish()
	return s
}

func TestTransitionFiniteStates(t *testing.T) {
	for kind := kindDefault; kind <= kindGlitch; kind++ {
		for dir := DirDefault; dir <= DirDown; dir++ {
			for _, forward := range []bool{false, true} {
				for w := 1; w <= 7; w++ {
					for h := 1; h <= 3; h++ {
						for _, p := range []float64{0, 1} {
							a, b := propertyFrame(w, h, true), propertyFrame(w, h, false)
							tr := Transition{kind: kind, dir: dir}.resolve()
							want := b.toGrid()
							if p == 0 && tr.kind != kindNone {
								want.release()
								want = a.toGrid()
							}
							mixTransition(tr, a, b, p, forward, testTheme)
							got := b.toGrid()
							if !reflect.DeepEqual(got.Cells, want.Cells) {
								t.Fatalf("kind=%d dir=%d forward=%v size=%dx%d p=%g: endpoint changed", kind, dir, forward, w, h, p)
							}
							got.release()
							want.release()
							a.Release()
							b.Release()
						}
					}
				}
			}
		}
	}
}

func FuzzTransitionReplay(f *testing.F) {
	for kind := kindDefault; kind <= kindGlitch; kind++ {
		for dir := DirDefault; dir <= DirDown; dir++ {
			f.Add(uint8(kind), uint8(dir), uint8(7), uint8(3), uint8(127), true)
		}
	}
	f.Fuzz(func(t *testing.T, kb, db, wb, hb, pb uint8, forward bool) {
		tr := Transition{kind: transitionKind(kb % uint8(kindGlitch+1)), dir: Direction(db % 5)}.resolve()
		w, h, p := int(wb%12)+1, int(hb%8)+1, float64(pb)/255
		a, b, replay := propertyFrame(w, h, true), propertyFrame(w, h, false), propertyFrame(w, h, false)
		defer a.Release()
		defer b.Release()
		defer replay.Release()
		source := a.toGrid()
		defer source.release()
		pixels := append([]RGB(nil), a.Px.Pix...)
		mixTransition(tr, a, b, p, forward, testTheme)
		mixTransition(tr, a, replay, p, forward, testTheme)
		g, again, after := b.toGrid(), replay.toGrid(), a.toGrid()
		defer g.release()
		defer again.release()
		defer after.release()
		if !reflect.DeepEqual(g.Cells, again.Cells) || !reflect.DeepEqual(b.Px.Pix, replay.Px.Pix) {
			t.Fatal("transition did not replay identically")
		}
		if !reflect.DeepEqual(source.Cells, after.Cells) || !reflect.DeepEqual(pixels, a.Px.Pix) {
			t.Fatal("transition mutated source frame")
		}
		for _, c := range b.Px.Pix {
			for _, v := range []float32{c.R, c.G, c.B} {
				if !finite(float64(v)) || v < -1e-4 || v > 255.001 {
					t.Fatalf("invalid transition color %v", c)
				}
			}
		}
	})
}

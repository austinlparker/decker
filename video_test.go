package decker

import (
	"bytes"
	"testing"
)

// countWriter counts frames without keeping them.
type countWriter struct{ frames, size int }

func (w *countWriter) Write(b []byte) (int, error) {
	if len(b) != w.size {
		panic("partial frame")
	}
	w.frames++
	return len(b), nil
}

// The video has exactly the frames its timing says, every one full size,
// and slides are really drawn into them (not left blank).
func TestVideoFrames(t *testing.T) {
	d := testDeck()
	slides := d.Slides
	o := videoOptions{width: 320, height: 180, fps: 10, hold: 0.5, first: 0, last: 3}
	want := 0
	for _, s := range slides[:4] {
		for step := 0; step < s.steps(); step++ {
			dur := videoTiming(s, step, o.hold)
			for tt := 0.0; tt < dur-0.05; tt += 0.1 {
				want++
			}
		}
	}
	w := &countWriter{size: 3 * o.width * o.height}
	if err := writeVideoFrames(d, o, w); err != nil {
		t.Fatal(err)
	}
	if w.frames != want {
		t.Fatalf("%d frames, want %d", w.frames, want)
	}
}

func TestVideoFramesAreDrawn(t *testing.T) {
	var last []byte
	o := videoOptions{width: 320, height: 180, fps: 5, hold: 0.4, first: 0, last: 0}
	err := writeVideoFrames(testDeck(), o, writerFunc(func(b []byte) (int, error) {
		last = append(last[:0], b...)
		return len(b), nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	bg := testTheme.Background.q()
	blank := bytes.Repeat(bg[:], o.width*o.height)
	if bytes.Equal(last, blank) {
		t.Fatal("the title slide's last frame is empty")
	}
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(b []byte) (int, error) { return f(b) }

package decker

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

// Rendering the deck to a video (-video talk.mp4): every slide and build
// step in order, each held for a fixed time, with entrance animations and
// slide transitions, piped as raw frames into ffmpeg. Frames come straight
// from the pixel canvas, so a 1920×1080 video is drawn on a 1920×540-cell
// canvas at one video pixel per canvas pixel.

type videoOptions struct {
	path          string
	width, height int // in pixels; height must be even
	fps           int
	hold          float64 // seconds each build step stays, unless the slide sets Hold
	first, last   int     // slides to include, 0-based
}

// videoTiming returns how long step (0-based) of s stays on screen. The
// first step also gets the time its entrance transition takes.
func videoTiming(s Slide, step int, hold float64) float64 {
	if s.Hold > 0 {
		hold = s.Hold
	}
	if step == 0 {
		hold += TransitionDuration
	}
	return hold
}

func renderVideo(d *Deck, o videoOptions) error {
	if o.width <= 0 || o.height <= 0 || o.height%2 != 0 {
		return fmt.Errorf("video size must be positive with an even height, got %dx%d", o.width, o.height)
	}
	o.last = min(o.last, len(d.Slides)-1)
	if o.first < 0 || o.first > o.last {
		return fmt.Errorf("no slides to render")
	}
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y",
		"-f", "rawvideo", "-pix_fmt", "rgb24", "-s", fmt.Sprintf("%dx%d", o.width, o.height),
		"-r", fmt.Sprint(o.fps), "-i", "-",
		// H.264 in the colors every player expects: BT.709, 4:2:0.
		"-vf", "scale=out_color_matrix=bt709:out_range=tv",
		"-c:v", "libx264", "-preset", "medium", "-crf", "16", "-tune", "animation",
		"-pix_fmt", "yuv420p", "-colorspace", "bt709", "-color_primaries", "bt709", "-color_trc", "bt709",
		"-movflags", "+faststart", o.path)
	cmd.Stderr = os.Stderr
	pipe, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting ffmpeg (is it installed?): %w", err)
	}
	werr := writeVideoFrames(d, o, pipe)
	pipe.Close()
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("ffmpeg: %w", err)
	}
	return werr
}

// writeVideoFrames draws every frame and writes it to w as packed RGB.
func writeVideoFrames(d *Deck, o videoOptions, w io.Writer) error {
	slides := d.Slides
	cw, ch := o.width, o.height/2 // canvas size in cells
	frame := make([]byte, 3*o.width*o.height)
	from := make([]byte, len(frame)) // the previous slide's last frame, for transitions
	var got *Pixels
	captureFrame = func(p *Pixels) {
		if p.W == o.width && p.H == o.height {
			toRGB24(p, frame)
			got = p
		}
	}
	defer func() { captureFrame = nil }()

	dt := 1 / float64(o.fps)
	start, frames := time.Now(), 0
	for i := o.first; i <= o.last; i++ {
		s := slides[i]
		kind := s.Transition
		if kind == TransitionDefault {
			kind = DefaultTransition
		}
		fmt.Fprintf(os.Stderr, "\rslide %d/%d  %s", i+1, o.last+1, padRight(s.Title, 40))
		slideT := 0.0
		for step := 0; step < s.steps(); step++ {
			dur := videoTiming(s, step, o.hold)
			for t := 0.0; t < dur-dt/2; t += dt {
				got = nil
				out := renderSlide(s, Ctx{W: cw, H: ch, T: slideT + t, Step: step, StepT: t, Theme: d.Theme})
				if got == nil {
					// The slide drew without a scene (or panicked): decode its text.
					toRGB24(framePixels(out, cw, ch, d.Theme), frame)
				}
				if step == 0 && i > o.first && t < TransitionDuration && kind != TransitionNone {
					blendTransition(kind, from, frame, o.width, o.height, t/TransitionDuration, d.Theme)
				}
				if n, err := w.Write(frame); err != nil {
					return err
				} else if n != len(frame) {
					return io.ErrShortWrite
				}
				frames++
			}
			slideT += dur
		}
		copy(from, frame)
	}
	secs := float64(frames) / float64(o.fps)
	fmt.Fprintf(os.Stderr, "\r%d frames, %d:%02d of video, in %v%s\n",
		frames, int(secs)/60, int(secs)%60, time.Since(start).Round(time.Second), padRight("", 40))
	return nil
}

// toRGB24 packs a canvas into 3 bytes per pixel.
func toRGB24(p *Pixels, dst []byte) {
	for i, c := range p.Pix {
		q := c.q()
		dst[3*i], dst[3*i+1], dst[3*i+2] = q[0], q[1], q[2]
	}
}

// blendTransition mixes the outgoing frame `from` into `to` in place, like
// composeTransition does for terminal cells. p runs from 0 (all from) to 1
// (all to). Videos always move forward.
func blendTransition(kind Transition, from, to []byte, w, h int, p float64, t *Theme) {
	e := EaseInOutCubic(p)
	switch kind {
	case TransitionPush: // the old slide slides out to the left
		off := int(float64(w) * e)
		row := make([]byte, 3*w)
		for y := 0; y < h; y++ {
			r := y * 3 * w
			copy(row, from[r+3*off:r+3*w])
			copy(row[3*(w-off):], to[r:r+3*off])
			copy(to[r:r+3*w], row)
		}
	case TransitionDissolve: // blocks the size of the deck's cells flip at random
		bs := max(w/400, 1)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				if Hash01(x/bs, y/(2*bs), 7) >= p {
					k := 3 * (y*w + x)
					copy(to[k:k+3], from[k:k+3])
				}
			}
		}
	case TransitionWipe: // an accent band sweeps left to right
		band := float64(w) / 60
		edge := -band + (float64(w)+2*band)*e
		acc, bg := t.Accent, t.Background
		for x := 0; x < w; x++ {
			d := edge - float64(x) // how far behind the edge this column is
			var col [3]uint8
			switch {
			case d >= band:
				continue // already the new slide
			case d >= 0:
				col = Mix(acc, bg, d/band).q()
			}
			for y := 0; y < h; y++ {
				k := 3 * (y*w + x)
				if d < 0 {
					copy(to[k:k+3], from[k:k+3])
				} else {
					to[k], to[k+1], to[k+2] = col[0], col[1], col[2]
				}
			}
		}
	}
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s[:n]
	}
	return s + fmt.Sprintf("%*s", n-len(s), "")
}

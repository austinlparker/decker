package decker

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

type videoOptions struct {
	path          string
	width, height int // in pixels; height must be even
	fps           int
	hold          float64 // seconds each build step stays, unless the slide sets Hold
	first, last   int     // slides to include, 0-based
}

// videoTiming is how long step (0-based) of s stays on screen; step 0 adds the
// entrance transition's time.
func videoTiming(s Slide, step int, hold float64) float64 {
	if s.Hold > 0 {
		hold = s.Hold
	}
	if step == 0 {
		hold += s.Transition.Duration()
	}
	return hold
}

// renderVideo draws every slide and build step in order, with entrance
// animations and transitions, and pipes rgb24 frames into ffmpeg (on PATH).
// One video pixel is one canvas pixel: 1920×1080 is a 1920×540-cell canvas.
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

// writeVideoFrames is renderVideo without ffmpeg, so tests can read the frames.
func writeVideoFrames(d *Deck, o videoOptions, w io.Writer) error {
	cw, ch := o.width, o.height/2 // canvas size in cells
	frame := make([]byte, 3*o.width*o.height)

	// from is the previous slide's last frame, drawn again unfinished so a
	// morph can move its placed elements.
	var from *Scene
	defer func() {
		if from != nil {
			from.Release()
		}
	}()
	var lastSlide Slide
	var lastCtx Ctx
	dt := 1 / float64(o.fps)
	start, frames := time.Now(), 0
	for i := o.first; i <= o.last; i++ {
		s := d.Slides[i]
		kind, tdur := s.Transition.resolve(), s.Transition.Duration()
		fmt.Fprintf(os.Stderr, "\rslide %d/%d  %-40.40s", i+1, o.last+1, s.Title)
		if from != nil {
			from.Release()
			from = nil
		}
		if i > o.first {
			from = drawSlide(lastSlide, lastCtx)
		}
		slideT := 0.0
		for step := 0; step < s.steps(); step++ {
			dur := videoTiming(s, step, o.hold)
			for t := 0.0; t < dur-dt/2; t += dt {
				c := Ctx{W: cw, H: ch, T: slideT + t, Step: step, StepT: t, Theme: d.Theme}.at(d.Slides, i)
				sc := drawSlide(s, c)
				if step == 0 && from != nil && t < tdur {
					mixTransition(kind, from, sc, t/tdur, true, d.Theme)
				}
				sc.finish()
				toRGB24(sc.Px, frame)
				sc.Release()
				lastSlide, lastCtx = s, c
				if _, err := w.Write(frame); err != nil {
					return err
				}
				frames++
			}
			slideT += dur
		}
	}
	secs := float64(frames) / float64(o.fps)
	fmt.Fprintf(os.Stderr, "\r%d frames, %d:%02d of video, in %v%40s\n",
		frames, int(secs)/60, int(secs)%60, time.Since(start).Round(time.Second), "")
	return nil
}

func toRGB24(p *Pixels, dst []byte) {
	for i, c := range p.Pix {
		q := c.q()
		dst[3*i], dst[3*i+1], dst[3*i+2] = q[0], q[1], q[2]
	}
}

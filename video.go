package decker

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

// videoOptions are the settings of one renderVideo run.
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

// renderVideo draws every slide and build step in order, each held for a
// fixed time, with entrance animations and slide transitions, and pipes the
// raw rgb24 frames into ffmpeg, which must be on PATH. Frames come straight
// from the pixel canvas at one video pixel per canvas pixel, so a 1920×1080
// video is drawn on a 1920×540-cell canvas.
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
	from := make([]byte, len(frame)) // the previous slide's last frame, for transitions
	drew := false
	captureFrame = func(p *Pixels) {
		if p.W == o.width && p.H == o.height {
			toRGB24(p, frame)
			drew = true
		}
	}
	defer func() { captureFrame = nil }()

	dt := 1 / float64(o.fps)
	start, frames := time.Now(), 0
	for i := o.first; i <= o.last; i++ {
		s := d.Slides[i]
		kind := s.Transition.resolve()
		fmt.Fprintf(os.Stderr, "\rslide %d/%d  %-40.40s", i+1, o.last+1, s.Title)
		slideT := 0.0
		for step := 0; step < s.steps(); step++ {
			dur := videoTiming(s, step, o.hold)
			for t := 0.0; t < dur-dt/2; t += dt {
				drew = false
				out := renderSlide(s, Ctx{W: cw, H: ch, T: slideT + t, Step: step, StepT: t, Theme: d.Theme})
				if !drew {
					// The slide drew without a scene (or panicked): decode its text.
					toRGB24(framePixels(out, cw, ch, d.Theme), frame)
				}
				if step == 0 && i > o.first && t < TransitionDuration {
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
	fmt.Fprintf(os.Stderr, "\r%d frames, %d:%02d of video, in %v%40s\n",
		frames, int(secs)/60, int(secs)%60, time.Since(start).Round(time.Second), "")
	return nil
}

// toRGB24 packs a canvas into 3 bytes per pixel.
func toRGB24(p *Pixels, dst []byte) {
	for i, c := range p.Pix {
		q := c.q()
		dst[3*i], dst[3*i+1], dst[3*i+2] = q[0], q[1], q[2]
	}
}

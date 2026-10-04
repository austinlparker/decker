package decker

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
)

// Main runs a deck from the command line: live in the terminal (the
// default), in dev mode, as the presenter view, or rendered to snapshots,
// a contact sheet or a video, as the flags say. Call it from the talk's
// main function. Register any flags of the talk's own before calling it;
// Main parses the command line.
func Main(d Deck) {
	if err := d.check(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	slideN := flag.Int("slide", 1, "start at this slide (1-based)")
	stepN := flag.Int("step", 1, "start at this build step (1-based)")
	fps := flag.Int("fps", 60, "animation frames per second")
	devMode := flag.Bool("dev", false, "rebuild and reload when .go files change")
	list := flag.Bool("list", false, "print slide titles and exit")
	snapshot := flag.Bool("snapshot", false, "print one frame of -slide to stdout and exit")
	at := flag.Float64("t", Settled, "with -snapshot: seconds since the slide appeared")
	width := flag.Int("w", 120, "with -snapshot: frame width")
	height := flag.Int("h", 36, "with -snapshot: frame height")
	pngOut := flag.String("png", "", "with -snapshot: write the frame as a PNG image instead of printing it")
	sheet := flag.String("sheet", "", "write every slide (last step, settled) into one PNG contact sheet and exit")
	shrink := flag.Int("shrink", 4, "with -sheet: shrink each frame by this factor (use 8 for very big terminals)")
	presenterMode := flag.Bool("presenter", false, "run the presenter view (notes, timer, next slide) and drive a deck running in another window")
	socket := flag.String("socket", defaultSocket(d.Name), "Unix socket linking the deck and the presenter view")
	talkLen := flag.Duration("length", 30*time.Minute, "with -presenter: the talk's length, for the timer and pace")
	previews := flag.String("previews", "auto", "with -presenter: draw slide previews as images (Ghostty, kitty) or cells; auto picks images when the terminal supports them")
	video := flag.String("video", "", "render the deck to this video file (MP4, needs ffmpeg) and exit; starts at -slide")
	videoSize := flag.String("size", "1920x1080", "with -video: the video's size in pixels")
	hold := flag.Float64("hold", 4, "with -video: seconds each build step stays on screen")
	until := flag.Int("until", 0, "with -video: the last slide to include (default: the end)")
	flag.Parse()

	if *video != "" {
		o := videoOptions{path: *video, fps: max(*fps, 1), hold: *hold, first: *slideN - 1, last: 1 << 30}
		if *until > 0 {
			o.last = *until - 1
		}
		if _, err := fmt.Sscanf(*videoSize, "%dx%d", &o.width, &o.height); err != nil {
			fmt.Fprintln(os.Stderr, "-size: want WIDTHxHEIGHT, like 1920x1080")
			os.Exit(1)
		}
		if err := renderVideo(&d, o); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	if *presenterMode {
		if err := runPresenter(&d, *socket, *talkLen, imagePreviews(*previews)); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	slides := d.Slides

	if *list {
		for i, s := range slides {
			plural := ""
			if s.steps() > 1 {
				plural = "s"
			}
			fmt.Printf("%3d  %s (%d step%s)\n", i+1, s.Title, s.steps(), plural)
		}
		return
	}

	if *sheet != "" {
		var frames []string
		for i, s := range slides {
			m := newModel(&d, i, s.steps()-1, 60, nil)
			m.w, m.h = *width, *height
			back := m.now.Add(-time.Duration(*at * float64(time.Second)))
			m.enter, m.stepStart = back, back
			frames = append(frames, m.View().Content)
		}
		if err := writeSheet(frames, *width, *height, 4, *shrink, *sheet, d.Theme); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	if *snapshot {
		m := newModel(&d, *slideN-1, *stepN-1, max(*fps, 1), nil)
		m.w, m.h = *width, *height
		// Put the slide (and its current step) *at seconds into its life.
		back := m.now.Add(-time.Duration(*at * float64(time.Second)))
		m.enter, m.stepStart = back, back
		frame := m.View().Content
		if *pngOut != "" {
			if err := writePNG(frame, m.w, m.h, *pngOut, d.Theme); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			return
		}
		fmt.Println(frame)
		return
	}

	var dev *devState
	if *devMode {
		bin, _ := filepath.Abs(filepath.Join(".slides", d.Name))
		var err error
		if dev, err = startDev(bin); err != nil {
			fmt.Fprintln(os.Stderr, "dev mode:", err)
			os.Exit(1)
		}
	}

	// The presenter view connects over the link; it's always on, and costs
	// nothing when nobody connects.
	var prog atomic.Pointer[tea.Program]
	link, err := listenLink(*socket, func(c linkCmd) {
		if p := prog.Load(); p != nil {
			p.Send(c)
		}
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// The deck draws its own frames (see termout.go); Bubble Tea handles
	// keys and the program loop.
	live, err := startLiveTerminal(d.Theme)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	m := newModel(&d, *slideN-1, *stepN-1, max(*fps, 1), dev)
	m.link = link
	m.live = live.writer
	p := tea.NewProgram(m, tea.WithoutRenderer())
	prog.Store(p)
	live.watchSize(p)
	final, err := p.Run()
	live.close()
	link.Close()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if m, ok := final.(model); ok && m.execOnQuit != nil {
		// Dev mode rebuilt us: become the new binary, on the same slide. A
		// connected presenter view sees the link drop and reconnects.
		args := append(m.execOnQuit, "-socket", *socket)
		if err := syscall.Exec(args[0], args, os.Environ()); err != nil {
			fmt.Fprintln(os.Stderr, "restart failed:", err)
			os.Exit(1)
		}
	}
}

// check reports what's missing from a deck before anything runs.
func (d *Deck) check() error {
	switch {
	case d.Name == "":
		return fmt.Errorf("deck: Name is required")
	case d.Theme == nil:
		return fmt.Errorf("deck: Theme is required")
	case d.Theme.Display == nil || d.Theme.Body == nil || d.Theme.Mono == nil:
		return fmt.Errorf("deck: Theme needs Display, Body and Mono fonts")
	case len(d.Slides) == 0:
		return fmt.Errorf("deck: no slides")
	}
	return nil
}

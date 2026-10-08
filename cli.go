package decker

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
)

type options struct {
	slide, step, fps     int
	dev, list            bool
	snapshot             bool
	at                   float64
	width, height        int
	png, sheet           string
	shrink               int
	presenter            bool
	presentationFontSize float64
	previews             string
	socket               string
	length               time.Duration
	video, size          string
	hold                 float64
	until                int
	review, sizes        string
	strict               bool
}

// Main runs a deck from the command line: live in the terminal by default, or
// as dev mode, presenter view, snapshot, contact sheet or video, as the flags
// say. It parses the command line, so register the talk's own flags first.
func Main(d Deck) {
	if err := run(&d); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(d *Deck) error {
	if err := d.check(); err != nil {
		return err
	}
	o := parseFlags(d.Name)
	switch {
	case o.video != "":
		return runVideo(d, o)
	case o.presenter:
		return runPresenter(d, o)
	case o.list:
		listSlides(d)
		return nil
	case o.review != "":
		return runReview(d, o)
	case o.sheet != "":
		return runSheet(d, o)
	case o.snapshot:
		return runSnapshot(d, o)
	}
	return runLive(d, o)
}

func parseFlags(name string) options {
	var o options
	flag.IntVar(&o.slide, "slide", 1, "start at this slide (1-based)")
	flag.IntVar(&o.step, "step", 1, "start at this build step (1-based)")
	flag.IntVar(&o.fps, "fps", 60, "animation frames per second")
	flag.BoolVar(&o.dev, "dev", false, "rebuild and reload when .go files change")
	flag.BoolVar(&o.list, "list", false, "print slide titles and exit")
	flag.BoolVar(&o.snapshot, "snapshot", false, "print one frame of -slide to stdout and exit")
	flag.Float64Var(&o.at, "t", Settled, "with -snapshot: seconds since the slide appeared")
	flag.IntVar(&o.width, "w", 120, "with -snapshot: frame width")
	flag.IntVar(&o.height, "h", 36, "with -snapshot: frame height")
	flag.StringVar(&o.png, "png", "", "with -snapshot: write the frame as a PNG image instead of printing it")
	flag.StringVar(&o.sheet, "sheet", "", "write every slide (last step, settled) into one PNG contact sheet and exit")
	flag.IntVar(&o.shrink, "shrink", 4, "with -sheet: shrink each frame by this factor (use 8 for very big terminals)")
	flag.BoolVar(&o.presenter, "presenter", false, "run the presenter view (notes, timer, next slide) and drive a deck running in another window")
	flag.StringVar(&o.socket, "socket", defaultSocket(name), "Unix socket linking the deck and the presenter view")
	flag.DurationVar(&o.length, "length", 30*time.Minute, "with -presenter: the talk's length, for the timer and pace")
	flag.Float64Var(&o.presentationFontSize, "presentation-font-size", 4, "with -presenter: font size in points for the Ghostty window opened with p")
	flag.StringVar(&o.previews, "previews", "auto", "with -presenter: slide previews as auto, image, or cells")
	flag.StringVar(&o.video, "video", "", "render the deck to this video file (MP4, needs ffmpeg) and exit; starts at -slide")
	flag.StringVar(&o.size, "size", "1920x1080", "with -video: the video's size in pixels")
	flag.Float64Var(&o.hold, "hold", 4, "with -video: seconds each build step stays on screen")
	flag.IntVar(&o.until, "until", 0, "with -video or -review: the last slide to include (default: the end)")
	flag.StringVar(&o.review, "review", "", "check every build of every slide at several sizes, write the issues to DIR/index.md and DIR/report.json, and exit; fails if it finds errors")
	flag.StringVar(&o.sizes, "sizes", "240x67,320x90,682x171", "with -review: frame sizes in cells, comma-separated")
	flag.BoolVar(&o.strict, "strict", false, "with -review: fail on warnings too")
	flag.Parse()
	o.fps = max(o.fps, 1)
	return o
}

func runVideo(d *Deck, o options) error {
	vo := videoOptions{path: o.video, fps: o.fps, hold: o.hold, first: o.slide - 1, last: 1 << 30}
	if o.until > 0 {
		vo.last = o.until - 1
	}
	if _, err := fmt.Sscanf(o.size, "%dx%d", &vo.width, &vo.height); err != nil {
		return errors.New("-size: want WIDTHxHEIGHT, like 1920x1080")
	}
	return renderVideo(d, vo)
}

func listSlides(d *Deck) {
	for i, s := range d.Slides {
		plural := ""
		if s.steps() > 1 {
			plural = "s"
		}
		sec := ""
		if name := sectionAt(d.Slides, i); name != "" {
			sec = "  [" + name + "]"
		}
		fmt.Printf("%3d  %s (%d step%s)%s\n", i+1, s.Title, s.steps(), plural, sec)
	}
}

// stillFrame renders slide idx at step, secs after it appeared, at w×h cells.
func stillFrame(d *Deck, idx, step int, secs float64, w, h int) *grid {
	return renderSlideGrid(d.Slides[idx], Ctx{W: w, H: h, T: secs, Step: step, StepT: secs, Theme: d.Theme}.at(d.Slides, idx))
}

func runSheet(d *Deck, o options) error {
	var frames []*grid
	for i, s := range d.Slides {
		frames = append(frames, stillFrame(d, i, s.steps()-1, o.at, o.width, o.height))
	}
	return writeSheet(frames, o.shrink, o.sheet)
}

func runReview(d *Deck, o options) error {
	sizes, err := parseSizes(o.sizes)
	if err != nil {
		return err
	}
	last := len(d.Slides)
	if o.until > 0 {
		last = min(o.until, last)
	}
	if o.slide < 1 || o.slide > last {
		return fmt.Errorf("-slide %d: the deck has slides 1 to %d", o.slide, last)
	}
	var slides []int
	for i := o.slide - 1; i < last; i++ {
		slides = append(slides, i)
	}
	r := reviewDeck(d, sizes, slides)
	r.writeText(os.Stdout)
	if err := r.save(o.review); err != nil {
		return err
	}
	fmt.Printf("Report: %s\n", filepath.Join(o.review, "index.md"))
	if e, w, _ := r.count(); e > 0 || o.strict && w > 0 {
		return fmt.Errorf("review found %s and %s", plural(e, "error", "errors"), plural(w, "warning", "warnings"))
	}
	return nil
}

func runSnapshot(d *Deck, o options) error {
	if o.slide < 1 || o.slide > len(d.Slides) {
		return fmt.Errorf("-slide %d: the deck has slides 1 to %d", o.slide, len(d.Slides))
	}
	if n := d.Slides[o.slide-1].steps(); o.step < 1 || o.step > n {
		return fmt.Errorf("-step %d: slide %d has steps 1 to %d", o.step, o.slide, n)
	}
	g := stillFrame(d, o.slide-1, o.step-1, o.at, o.width, o.height)
	if o.png != "" {
		return writePNG(g, o.png)
	}
	fmt.Println(g.String())
	return nil
}

func runLive(d *Deck, o options) error {
	var dev *devState
	if o.dev {
		bin, _ := filepath.Abs(filepath.Join(".slides", d.Name))
		var err error
		if dev, err = startDev(bin); err != nil {
			return fmt.Errorf("dev mode: %w", err)
		}
	}

	// The link is always on; it costs nothing until a presenter view connects.
	var prog atomic.Pointer[tea.Program]
	link, err := listenLink(o.socket, func(c linkCmd) {
		if p := prog.Load(); p != nil {
			p.Send(c)
		}
	})
	if err != nil {
		return err
	}
	// The deck draws its own frames; Bubble Tea only handles keys.
	live, err := startLiveTerminal(d.Theme)
	if err != nil {
		return err
	}
	m := newModel(d, o.slide-1, o.step-1, o.fps, dev)
	m.link = link
	m.live = live.writer
	p := tea.NewProgram(m, tea.WithoutRenderer())
	prog.Store(p)
	live.watchSize(p)
	_, err = p.Run()
	live.close()
	link.close()
	if err != nil {
		return err
	}
	if dev != nil && dev.restart != nil {
		// Dev mode rebuilt us: exec the new binary on the same slide; a
		// presenter view reconnects.
		if err := execRestart(dev.restart, o.socket); err != nil {
			return fmt.Errorf("restart failed: %w", err)
		}
	}
	return nil
}

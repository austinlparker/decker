package decker

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/alecthomas/kong"
)

// Main runs a deck from the command line: live in the terminal by default,
// or the command given (present, list, review, handout, snapshot, sheet,
// video). It exits on a bad command line or a failed command; `--help`
// lists them all.
func Main(d Deck) {
	k, ctx, err := parseArgs(&d, os.Args[1:])
	if err == nil {
		err = ctx.Run(&d)
	}
	if k == nil {
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	k.FatalIfErrorf(err)
}

// cli is the command line every talk gets from Main.
type cli struct {
	Live     liveCmd     `cmd:"" default:"withargs" help:"Present in this terminal. The default: flags alone, like --dev, run it."`
	Present  presentCmd  `cmd:"" help:"Show notes, a timer and previews, and drive the deck in another window."`
	List     listCmd     `cmd:"" help:"Print each slide's title, builds and section."`
	Review   reviewCmd   `cmd:"" help:"Check every build of every slide at 240x67, 320x90 and 682x171 for clipped, unreadable or overlapping content, and write a report with pictures to DIR. Fails on errors."`
	Handout  handoutCmd  `cmd:"" help:"Write DIR/handout.md, with each slide's thumbnail, notes and sources, and the thumbnails."`
	Snapshot snapshotCmd `cmd:"" help:"Print one build of one slide, or write it as a PNG."`
	Sheet    sheetCmd    `cmd:"" help:"Write every slide, at its last build, into one PNG contact sheet."`
	Video    videoCmd    `cmd:"" help:"Render the deck to an MP4 video (needs ffmpeg)."`
}

// parseArgs parses args, the command line after the program name, for d. It
// returns no parser if the deck itself is invalid.
func parseArgs(d *Deck, args []string) (*kong.Kong, *kong.Context, error) {
	if err := d.check(); err != nil {
		return nil, nil, err
	}
	k, err := kong.New(&cli{},
		kong.Name(d.Name),
		kong.Description("A talk built with decker. With no command, it presents in this terminal."),
		kong.Vars{"socket": defaultSocket(d.Name), "settled": strconv.FormatFloat(Settled, 'g', -1, 64)},
		kong.ShortUsageOnError(),
		// -h is a frame's height, as -w is its width; help is --help.
		kong.PostBuild(func(k *kong.Kong) error {
			k.Model.HelpFlag.Short = 0
			return nil
		}),
	)
	if err != nil {
		return nil, nil, err
	}
	if a := oldStyleArg(args); a != "" {
		return k, nil, fmt.Errorf("%s: decker's modes are commands now and its long flags take two dashes, "+
			"as in `review DIR` or `snapshot --slide 3`; see --help", a)
	}
	ctx, err := k.Parse(args)
	return k, ctx, err
}

// oldStyleArg returns the first argument written as decker's command line
// was before it had commands, a single dash and a long name (-review,
// -slide=3), which would otherwise read as a run of short flags.
func oldStyleArg(args []string) string {
	for _, a := range args {
		if a == "--" {
			break
		}
		name, _, _ := strings.Cut(a, "=")
		if len(name) > 2 && name[0] == '-' && name[1] != '-' && unicode.IsLetter(rune(name[1])) {
			return a
		}
	}
	return ""
}

// position is where a command starts, counted from 1 as the command line
// counts.
type position struct {
	Slide int `default:"1" help:"Slide to start at, from 1."`
	Step  int `default:"1" help:"Build to start at, from 1."`
}

// deckFlags are how the live deck runs, here or in the window the presenter
// view opens.
type deckFlags struct {
	FPS    int    `name:"fps" default:"60" help:"Animation frames per second."`
	Dev    bool   `help:"Rebuild and reload when the talk's Go files change, keeping the slide and build."`
	Socket string `default:"${socket}" help:"Unix socket linking the deck and the presenter view."`
}

func (f deckFlags) check() error {
	if f.FPS < 1 {
		return fmt.Errorf("--fps %d: want at least 1", f.FPS)
	}
	return nil
}

// args are the flags that run a deck the same way, but for its socket.
func (f deckFlags) args() []string {
	a := []string{"--fps=" + strconv.Itoa(f.FPS)}
	if f.Dev {
		a = append(a, "--dev")
	}
	return a
}

// frameFlags are the size and moment of a still frame.
type frameFlags struct {
	Width  int     `short:"w" default:"120" help:"Frame width in cells."`
	Height int     `short:"h" default:"36" help:"Frame height in cells."`
	Time   float64 `short:"t" default:"${settled}" help:"Seconds since the slide and its build appeared; the default shows it settled."`
}

func (f frameFlags) check() error {
	if f.Width < 1 || f.Height < 1 {
		return fmt.Errorf("-w and -h must be positive, not %dx%d", f.Width, f.Height)
	}
	return nil
}

type liveCmd struct {
	position
	deckFlags
}

func (c liveCmd) Validate() error { return c.check() }

func (c liveCmd) Run(d *Deck) error {
	var dev *devState
	if c.Dev {
		bin, _ := filepath.Abs(filepath.Join(".slides", d.Name))
		var err error
		if dev, err = startDev(bin); err != nil {
			return fmt.Errorf("dev mode: %w", err)
		}
	}

	// The link is always on; it costs nothing until a presenter view connects.
	var prog atomic.Pointer[tea.Program]
	link, err := listenLink(c.Socket, func(c linkCmd) {
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
	m := newModel(d, c.Slide-1, c.Step-1, c.FPS, dev)
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
		if err := execRestart(dev.restart, c.Socket); err != nil {
			return fmt.Errorf("restart failed: %w", err)
		}
	}
	return nil
}

// presentCmd is the presenter view; its deck flags are for the deck window
// it opens with p.
type presentCmd struct {
	position
	deckFlags
	Length               time.Duration `default:"30m" help:"The talk's length, for the timer and pace."`
	PresentationFontSize float64       `default:"4" help:"Font size in points for the Ghostty window opened with p."`
	Previews             string        `enum:"auto,image,cells" default:"auto" help:"Slide previews as images or cells; auto picks images in Ghostty or kitty outside tmux and zellij."`
}

func (c presentCmd) Validate() error {
	if f := c.PresentationFontSize; f <= 0 || math.IsNaN(f) || math.IsInf(f, 0) {
		return errors.New("--presentation-font-size: want a positive, finite size in points")
	}
	return c.check()
}

type listCmd struct{}

func (listCmd) Run(d *Deck) error {
	listSlides(d)
	return nil
}

func listSlides(d *Deck) {
	for i, s := range d.Slides {
		sec := ""
		if name := d.Section(i); name != "" {
			sec = "  [" + name + "]"
		}
		fmt.Printf("%3d  %s (%s)%s\n", i+1, s.Title, plural(d.Steps(i), "step"), sec)
	}
}

type reviewCmd struct {
	Dir string `arg:"" help:"Directory for the report and its pictures."`
}

func (c reviewCmd) Run(d *Deck) error {
	r, err := reviewDeck(d, reviewSizes, c.Dir)
	if err != nil {
		return err
	}
	r.writeText(os.Stdout)
	if err := r.save(c.Dir); err != nil {
		return err
	}
	fmt.Printf("Report: %s\n", filepath.Join(c.Dir, "index.md"))
	if n := r.errors(); n > 0 {
		return fmt.Errorf("review found %s", plural(n, "error"))
	}
	return nil
}

type snapshotCmd struct {
	position
	frameFlags
	PNG string `name:"png" placeholder:"FILE" help:"Write the frame as a PNG image instead of printing it."`
}

func (c snapshotCmd) Validate() error { return c.frameFlags.check() }

func (c snapshotCmd) Run(d *Deck) error {
	if c.Slide < 1 || c.Slide > len(d.Slides) {
		return fmt.Errorf("--slide %d: the deck has slides 1 to %d", c.Slide, len(d.Slides))
	}
	if n := d.Slides[c.Slide-1].steps(); c.Step < 1 || c.Step > n {
		return fmt.Errorf("--step %d: slide %d has builds 1 to %d", c.Step, c.Slide, n)
	}
	g := d.still(c.Slide-1, c.Step-1, c.Time, c.Width, c.Height)
	if c.PNG != "" {
		return writePNG(g, c.PNG)
	}
	fmt.Println(g.String())
	return nil
}

type sheetCmd struct {
	File string `arg:"" help:"The PNG to write."`
	frameFlags
	Shrink int `default:"4" help:"Shrink each frame by this factor; 8 for very big terminals."`
}

func (c sheetCmd) Validate() error {
	if c.Shrink < 1 {
		return fmt.Errorf("--shrink %d: want at least 1", c.Shrink)
	}
	return c.frameFlags.check()
}

func (c sheetCmd) Run(d *Deck) error {
	var frames []*grid
	for i := range d.Slides {
		frames = append(frames, d.still(i, d.Steps(i)-1, c.Time, c.Width, c.Height))
	}
	return writeSheet(frames, c.Shrink, c.File)
}

type videoCmd struct {
	File  string  `arg:"" help:"The MP4 to write."`
	Slide int     `default:"1" help:"First slide, from 1."`
	Until int     `help:"Last slide, from 1; the default is the end."`
	Size  string  `default:"1920x1080" help:"The video's size in pixels, WIDTHxHEIGHT."`
	Hold  float64 `default:"4" help:"Seconds each build stays on screen, unless its slide sets Hold."`
	FPS   int     `name:"fps" default:"60" help:"Frames per second."`
}

func (c videoCmd) Validate() error {
	if c.FPS < 1 {
		return fmt.Errorf("--fps %d: want at least 1", c.FPS)
	}
	return nil
}

func (c videoCmd) Run(d *Deck) error {
	vo := videoOptions{path: c.File, fps: c.FPS, hold: c.Hold, first: c.Slide - 1, last: 1 << 30}
	if c.Until > 0 {
		vo.last = c.Until - 1
	}
	if _, err := fmt.Sscanf(c.Size, "%dx%d", &vo.width, &vo.height); err != nil {
		return errors.New("--size: want WIDTHxHEIGHT, like 1920x1080")
	}
	return renderVideo(d, vo)
}

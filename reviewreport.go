package decker

import (
	"encoding/json"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
)

// reviewRun is one -review: what was checked, what was found, and the
// images written for it, relative to its directory.
type reviewRun struct {
	deck   string
	sizes  [][2]int
	slides []int
	builds int
	issues []Issue
	frames map[[2]int][]string // by slide and build
	sheets []string
}

// reviewDeck reviews the deck for the command line. With dir set it writes
// images there as it goes: an annotated frame for each build with issues
// (frames "issues"), or for every build ("all"), or none ("none"), and a
// contact sheet of every build per size.
func reviewDeck(d *Deck, sizes [][2]int, slides []int, dir, frames string) (reviewRun, error) {
	r := reviewRun{deck: d.Name, sizes: sizes, slides: slides, frames: map[[2]int][]string{}}
	for _, i := range slides {
		r.builds += d.Steps(i)
	}
	digits := max(2, len(strconv.Itoa(len(d.Slides))))
	tiles := map[[2]int][]sheetTile{}
	var err error
	save := func(img image.Image, name string) {
		if err == nil {
			err = savePNG(img, filepath.Join(dir, name))
		}
	}
	if dir != "" && frames != "none" {
		if err := os.MkdirAll(filepath.Join(dir, "frames"), 0o755); err != nil {
			return r, err
		}
	}
	d.reviewEach(ReviewOptions{Sizes: sizes, Slides: slides}, func(f *reviewedFrame) {
		r.issues = append(r.issues, f.issues...)
		if dir == "" || f.sc == nil {
			return
		}
		title := d.Slides[f.slide].Title
		size := [2]int{f.w, f.h}
		tiles[size] = append(tiles[size], tile(f, title))
		if frames == "all" || frames == "issues" && len(f.issues) > 0 {
			name := fmt.Sprintf("frames/%0*d-%d-%s.png", digits, f.slide+1, f.step+1, sizeName(size))
			save(annotate(f, title), name)
			k := [2]int{f.slide, f.step}
			r.frames[k] = append(r.frames[k], name)
		}
	})
	if dir == "" {
		return r, nil
	}
	for _, sz := range sizes {
		var found []Issue
		for _, is := range r.issues {
			if is.W == sz[0] && is.H == sz[1] {
				found = append(found, is)
			}
		}
		caption := fmt.Sprintf("%s  ·  %s  ·  %s", r.deck, sizeName(sz), plural(len(tiles[sz]), "build"))
		if c := issueCounts(found); c != "" {
			caption += "  ·  " + c
		}
		name := "sheet-" + sizeName(sz) + ".png"
		save(reviewSheet(tiles[sz], caption), name)
		r.sheets = append(r.sheets, name)
	}
	return r, err
}

// count returns how many issues have each severity.
func (r reviewRun) count() (errors, warnings, infos int) {
	for _, is := range r.issues {
		switch is.Severity {
		case SeverityError:
			errors++
		case SeverityWarning:
			warnings++
		default:
			infos++
		}
	}
	return errors, warnings, infos
}

func sizeName(sz [2]int) string { return fmt.Sprintf("%dx%d", sz[0], sz[1]) }

func (r reviewRun) sizeList() string {
	names := make([]string, len(r.sizes))
	for i, sz := range r.sizes {
		names[i] = sizeName(sz)
	}
	return strings.Join(names, ", ")
}

// summary is the run in one line.
func (r reviewRun) summary() string {
	checked := fmt.Sprintf("%s of %s at %s", plural(r.builds, "build"), plural(len(r.slides), "slide"), r.sizeList())
	if found := issueCounts(r.issues); found != "" {
		return found + " in " + checked + "."
	}
	return "No issues in " + checked + "."
}

// issueGroup is one issue found the same at several sizes.
type issueGroup struct {
	Issue
	sizes []string
}

// groups folds issues that differ only in size into one, keeping order.
func (r reviewRun) groups() []issueGroup {
	type key struct {
		slide, step int
		sev         Severity
		code, msg   string
	}
	var out []issueGroup
	at := map[key]int{}
	for _, is := range r.issues {
		k := key{is.Slide, is.Step, is.Severity, is.Code, is.Msg}
		if i, ok := at[k]; ok {
			out[i].sizes = append(out[i].sizes, sizeName([2]int{is.W, is.H}))
			continue
		}
		at[k] = len(out)
		out = append(out, issueGroup{is, []string{sizeName([2]int{is.W, is.H})}})
	}
	return out
}

// writeText prints the issues, one per line, then the summary.
func (r reviewRun) writeText(w io.Writer) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, g := range r.groups() {
		fmt.Fprintf(tw, "%d.%d\t%s\t%s\t%s\t%s\t%s\n", g.Slide+1, g.Step+1, g.Title, strings.Join(g.sizes, ","), g.Severity, g.Code, g.Msg)
	}
	tw.Flush()
	fmt.Fprintln(w, r.summary())
}

// writeMarkdown writes the report a person or an agent reads first: the
// summary, then each slide with issues, build by build.
func (r reviewRun) writeMarkdown(w io.Writer) {
	fmt.Fprintf(w, "# Review: %s\n\n%s Each build is checked settled, as it looks once its animations finish.\n", r.deck, r.summary())
	if len(r.sheets) > 0 {
		fmt.Fprintf(w, "\nEvery build at a glance, framed in the color of its worst issue (red errors, amber warnings, blue notes):")
		for _, s := range r.sheets {
			fmt.Fprintf(w, " [%s](%s)", strings.TrimSuffix(strings.TrimPrefix(s, "sheet-"), ".png"), s)
		}
		fmt.Fprintln(w)
	}
	slide, step := -1, -1
	endBuild := func() {
		for _, img := range r.frames[[2]int{slide, step}] {
			fmt.Fprintf(w, "\n![%d.%d at %s](%s)\n", slide+1, step+1, strings.TrimSuffix(img[strings.LastIndex(img, "-")+1:], ".png"), img)
		}
	}
	for _, g := range r.groups() {
		if g.Slide != slide || g.Step != step {
			if step >= 0 {
				endBuild()
			}
		}
		if g.Slide != slide {
			slide, step = g.Slide, -1
			fmt.Fprintf(w, "\n## %d. %s\n", g.Slide+1, g.Title)
		}
		if g.Step != step {
			step = g.Step
			fmt.Fprintf(w, "\n**Build %d**\n\n", g.Step+1)
		}
		fmt.Fprintf(w, "- **%s** `%s` at %s: %s\n", g.Severity, g.Code, strings.Join(g.sizes, ", "), g.Msg)
	}
	if step >= 0 {
		endBuild()
	}
}

// reviewJSON is report.json: slides and builds are 1-based, as on the
// command line.
type reviewJSON struct {
	Deck   string            `json:"deck"`
	Sizes  []string          `json:"sizes"`
	Slides int               `json:"slides"`
	Builds int               `json:"builds"`
	Issues []reviewIssueJSON `json:"issues"`
}

type reviewIssueJSON struct {
	Slide    int    `json:"slide"`
	Build    int    `json:"build"`
	Title    string `json:"title"`
	Size     string `json:"size"`
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Rect     [4]int `json:"rect"` // x, y, w, h in canvas pixels
	Message  string `json:"message"`
}

func (r reviewRun) writeJSON(w io.Writer) error {
	out := reviewJSON{Deck: r.deck, Slides: len(r.slides), Builds: r.builds, Issues: []reviewIssueJSON{}}
	for _, sz := range r.sizes {
		out.Sizes = append(out.Sizes, sizeName(sz))
	}
	for _, is := range r.issues {
		out.Issues = append(out.Issues, reviewIssueJSON{
			Slide: is.Slide + 1, Build: is.Step + 1, Title: is.Title, Size: sizeName([2]int{is.W, is.H}),
			Severity: is.Severity.String(), Code: is.Code, Message: is.Msg,
			Rect: [4]int{int(is.Rect.X), int(is.Rect.Y), int(is.Rect.W), int(is.Rect.H)},
		})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// save writes index.md and report.json into dir.
func (r reviewRun) save(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var md strings.Builder
	r.writeMarkdown(&md)
	if err := os.WriteFile(filepath.Join(dir, "index.md"), []byte(md.String()), 0o644); err != nil {
		return err
	}
	var js strings.Builder
	if err := r.writeJSON(&js); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "report.json"), []byte(js.String()), 0o644)
}

// parseSizes reads -sizes: WxH sizes in cells, comma-separated.
func parseSizes(s string) ([][2]int, error) {
	var out [][2]int
	for _, part := range strings.Split(s, ",") {
		var w, h int
		if _, err := fmt.Sscanf(strings.TrimSpace(part), "%dx%d", &w, &h); err != nil || w <= 0 || h <= 0 {
			return nil, fmt.Errorf("-sizes: want WxH sizes in cells, comma-separated, like 240x67,682x171; got %q", part)
		}
		out = append(out, [2]int{w, h})
	}
	return out, nil
}

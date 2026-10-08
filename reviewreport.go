package decker

import (
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
)

// reviewRun is one -review: what was checked, what was found, and the
// images written for it, relative to its directory.
type reviewRun struct {
	deck   string
	sizes  [][2]int
	slides int
	builds int
	issues []Issue
	frames map[[2]int][]string // by slide and build
	sheets []string
}

// reviewDeck reviews the deck for the command line, writing into dir as it
// goes an annotated frame for each build with issues and, per size, a
// contact sheet of every build.
func reviewDeck(d *Deck, sizes [][2]int, dir string) (reviewRun, error) {
	r := reviewRun{deck: d.Name, sizes: sizes, slides: len(d.Slides), builds: d.builds(), frames: map[[2]int][]string{}}
	if err := os.MkdirAll(filepath.Join(dir, "frames"), 0o755); err != nil {
		return r, err
	}
	tiles := map[[2]int][]sheetTile{}
	var err error
	save := func(img image.Image, name string) {
		if err == nil {
			err = savePNG(img, filepath.Join(dir, name))
		}
	}
	d.review(sizes, func(f *reviewedFrame) {
		r.issues = append(r.issues, f.issues...)
		title := d.Slides[f.slide].Title
		size := [2]int{f.w, f.h}
		tiles[size] = append(tiles[size], tile(f, title))
		if len(f.issues) > 0 {
			name := fmt.Sprintf("frames/%s-%d-%s.png", slideNumber(f.slide+1, len(d.Slides)), f.step+1, sizeName(size))
			save(annotate(f, title), name)
			k := [2]int{f.slide, f.step}
			r.frames[k] = append(r.frames[k], name)
		}
	})
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

// errors counts the issues that are errors.
func (r reviewRun) errors() int {
	n := 0
	for _, is := range r.issues {
		if is.Severity == SeverityError {
			n++
		}
	}
	return n
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
	checked := fmt.Sprintf("%s of %s at %s", plural(r.builds, "build"), plural(r.slides, "slide"), r.sizeList())
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
		fmt.Fprintf(w, "\nEvery build at a glance, framed in the color of its worst issue (red errors, amber warnings):")
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

// save writes index.md into dir.
func (r reviewRun) save(dir string) error {
	var md strings.Builder
	r.writeMarkdown(&md)
	return os.WriteFile(filepath.Join(dir, "index.md"), []byte(md.String()), 0o644)
}

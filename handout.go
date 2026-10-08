package decker

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type handoutCmd struct {
	Dir string `arg:"" help:"Directory for handout.md and the thumbnails."`
	frameFlags
}

func (c handoutCmd) Validate() error { return c.frameFlags.check() }

// Run writes the deck as a handout to rehearse from or hand out:
// handout.md with each slide's thumbnail, notes and sources, and the
// thumbnails beside it, each slide's final build at the frame's size and
// moment.
func (c handoutCmd) Run(d *Deck) error {
	if err := os.MkdirAll(c.Dir, 0o755); err != nil {
		return fmt.Errorf("handout: %w", err)
	}
	thumbs := thumbNames(len(d.Slides))
	for i, s := range d.Slides {
		g := d.still(i, s.steps()-1, c.Time, c.Width, c.Height)
		err := writePNG(g, filepath.Join(c.Dir, thumbs[i]))
		g.release()
		if err != nil {
			return fmt.Errorf("handout: %w", err)
		}
	}
	md := []byte(handoutMarkdown(d, thumbs))
	if err := os.WriteFile(filepath.Join(c.Dir, "handout.md"), md, 0o644); err != nil {
		return fmt.Errorf("handout: %w", err)
	}
	return nil
}

// thumbNames names n thumbnails 01.png, 02.png and on.
func thumbNames(n int) []string {
	names := make([]string, n)
	for i := range names {
		names[i] = slideNumber(i+1, n) + ".png"
	}
	return names
}

// slideNumber is the 1-based slide number n of a deck of last slides, for a
// file name: padded with zeros to the width of last, and to at least two
// digits, so the files sort in slide order.
func slideNumber(n, last int) string {
	return fmt.Sprintf("%0*d", max(len(strconv.Itoa(last)), 2), n)
}

// handoutMarkdown is handout.md, with thumbs[i] as slide i's image: the deck,
// then every slide with its notes and sources, then every source once with
// the slides that cite it.
func handoutMarkdown(d *Deck, thumbs []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n%s, %s.\n", mdText(d.Name), plural(len(d.Slides), "slide"), plural(d.builds(), "build"))

	// One entry per source, in order of first citation. A URL identifies a
	// source, whatever each slide calls it; without one, its label does.
	type citation struct {
		src    Source
		slides []int
	}
	var cited []*citation
	byKey := map[Source]*citation{}
	for i, s := range d.Slides {
		fmt.Fprintf(&b, "\n## %d. %s\n\n*", i+1, mdText(s.Title))
		if sec := sectionAt(d.Slides, i); sec != "" {
			b.WriteString(mdText(sec) + " · ")
		}
		fmt.Fprintf(&b, "%s*\n\n![Slide %d](%s)\n", plural(s.steps(), "build"), i+1, thumbs[i])
		if notes := strings.TrimSpace(s.Notes); notes != "" {
			b.WriteString("\n" + notes + "\n")
		}
		if len(s.Sources) > 0 {
			b.WriteString("\nSources:\n\n")
		}
		for _, src := range s.Sources {
			b.WriteString("- " + mdSource(src) + "\n")
			key := Source{URL: src.URL}
			if src.URL == "" {
				key.Label = src.Label
			}
			c := byKey[key]
			if c == nil {
				c = &citation{src: src}
				byKey[key] = c
				cited = append(cited, c)
			}
			if n := len(c.slides); n == 0 || c.slides[n-1] != i+1 {
				c.slides = append(c.slides, i+1)
			}
		}
	}

	if len(cited) > 0 {
		b.WriteString("\n## Sources\n\n")
	}
	for _, c := range cited {
		nums := make([]string, len(c.slides))
		for i, n := range c.slides {
			nums[i] = strconv.Itoa(n)
		}
		word := "slide"
		if len(nums) > 1 {
			word = "slides"
		}
		fmt.Fprintf(&b, "- %s — %s %s\n", mdSource(c.src), word, strings.Join(nums, ", "))
	}
	return b.String()
}

// plural is n and noun, with an s unless n is 1: "1 slide", "3 builds".
func plural(n int, noun string) string {
	if n != 1 {
		noun += "s"
	}
	return strconv.Itoa(n) + " " + noun
}

// mdText escapes the characters that would turn a title or label into
// Markdown formatting, and keeps it on one line.
var mdText = strings.NewReplacer(
	`\`, `\\`, "`", "\\`", "*", `\*`, "_", `\_`, "[", `\[`, "]", `\]`, "<", `\<`, "\n", " ",
).Replace

// mdSource is s as Markdown: a link when it has a URL. A URL with spaces or
// parentheses goes in angle brackets, which keep them from ending the link.
func mdSource(s Source) string {
	if s.URL == "" {
		return mdText(s.Label)
	}
	url := s.URL
	if strings.ContainsAny(url, " ()") {
		url = "<" + url + ">"
	}
	return "[" + mdText(s.name()) + "](" + url + ")"
}

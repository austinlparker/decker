package decker

import (
	"fmt"
	"strconv"
	"strings"
)

// deckNotes is a deck's Notes, parsed: what each slide's section says, and
// the posts a live thread makes (see Publisher).
type deckNotes struct {
	opener []notePost   // the posts before the first section, which start the thread
	slides []slideNotes // one per slide; zero past the last section
}

// slideNotes is one slide's section: the notes the speaker reads, without
// its post and alt blocks, and the posts.
type slideNotes struct {
	text  string
	posts []notePost
}

// notePost is a post block: its text, what it attaches and the alt text for
// that, and the line it starts on, for errors.
type notePost struct {
	text   string
	media  postMedia
	builds []int // 0-based, the builds an image post shows: the last for postFinal, every one for postBuilds unless listed
	alt    string
	line   int
}

// postMedia is what a post attaches of its slide.
type postMedia uint8

const (
	postFinal  postMedia = iota // the last build, settled, as an image
	postNone                    // text alone
	postBuilds                  // an image of each build, settled
	postClip                    // the slide entering and building, as a looping video
)

var postMedias = map[string]postMedia{"final": postFinal, "none": postNone, "builds": postBuilds, "clip": postClip}

// parseNotes splits md, a deck's Notes, into each slide's section. A
// level-two ATX heading starts the next slide's section and must be its title
// as written; headings inside fenced or indented code don't count, so a YAML
// or shell comment in a code sample is safe. A fenced block whose info
// string starts with "post" is a post, and an "alt" block right after one
// is its media's alt text; both are left out of the notes.
func parseNotes(md string, slides []Slide) (deckNotes, error) {
	dn := deckNotes{slides: make([]slideNotes, len(slides))}
	n := 0 // sections so far
	var body []string
	var posts []notePost
	var fence string    // the open code fence, "" outside one
	var fenceAt int     // the line it opened on
	var capture *string // the post or alt block being read, if any
	var captured []string
	flush := func() {
		if n == 0 {
			dn.opener = posts
		} else {
			dn.slides[n-1] = slideNotes{text: strings.TrimSpace(strings.Join(body, "\n")), posts: posts}
		}
		body, posts = body[:0], nil
	}
	for i, line := range strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n") {
		if fence == "" {
			if title, ok := noteHeading(line); ok {
				flush()
				switch {
				case n == len(slides):
					return deckNotes{}, fmt.Errorf("deck: Notes line %d: section %d, %q, is past the last slide (%d)",
						i+1, n+1, title, len(slides))
				case title != slides[n].Title:
					return deckNotes{}, fmt.Errorf("deck: Notes line %d: section %d is %q, but slide %d is %q; "+
						"Notes has one \"## Title\" section per slide, in order",
						i+1, n+1, title, n+1, slides[n].Title)
				}
				n++
				continue
			}
			if marker, info, ok := openFence(line); ok {
				fence, fenceAt = marker, i+1
				switch kind, opts, _ := strings.Cut(info, " "); kind {
				case "post":
					steps := 0
					if n > 0 {
						steps = slides[n-1].steps()
					}
					p, err := parsePost(opts, i+1, n-1, steps)
					if err != nil {
						return deckNotes{}, err
					}
					posts = append(posts, p)
					capture, captured = &posts[len(posts)-1].text, captured[:0]
					continue
				case "alt":
					if len(posts) == 0 || posts[len(posts)-1].alt != "" || posts[len(posts)-1].media == postNone {
						return deckNotes{}, fmt.Errorf("deck: Notes line %d: an alt block goes right after the post "+
							"whose media it describes, once", i+1)
					}
					capture, captured = &posts[len(posts)-1].alt, captured[:0]
					continue
				}
				body = append(body, line)
				continue
			}
		} else if fence = nextFence(fence, line); fence == "" && capture != nil {
			*capture = strings.TrimSpace(strings.Join(captured, "\n"))
			capture = nil
			continue
		}
		if capture != nil {
			captured = append(captured, line)
		} else {
			body = append(body, line)
		}
	}
	// Markdown runs an unclosed fence to the end, which here would quietly
	// take every later section's notes and posts with it.
	if fence != "" {
		return deckNotes{}, fmt.Errorf("deck: Notes line %d: a code block is never closed", fenceAt)
	}
	flush()
	return dn, nil
}

// parsePost reads a post block's options, the info string after "post":
// media=final|none|builds|clip, and for builds, builds=LIST, a
// comma-separated list of 1-based builds and ranges ("2-4,6"). slide is
// the section's, from 0, and steps its builds; a post before the first
// section (slide -1) opens the thread and attaches nothing.
func parsePost(opts string, line, slide, steps int) (notePost, error) {
	p := notePost{line: line}
	if slide < 0 {
		p.media = postNone
	}
	var builds string
	for _, f := range strings.Fields(opts) {
		k, v, _ := strings.Cut(f, "=")
		switch k {
		case "media":
			m, ok := postMedias[v]
			if !ok {
				return p, fmt.Errorf("deck: Notes line %d: media=%s: want final, none, builds or clip", line, v)
			}
			p.media = m
		case "builds":
			builds = v
		default:
			return p, fmt.Errorf("deck: Notes line %d: post option %q: want media= or builds=", line, f)
		}
	}
	switch {
	case slide < 0 && p.media != postNone:
		return p, fmt.Errorf("deck: Notes line %d: a post before the first slide's section has no "+
			"slide to show; give it media=none", line)
	case builds != "" && p.media != postBuilds:
		return p, fmt.Errorf("deck: Notes line %d: builds= goes with media=builds", line)
	case p.media == postFinal:
		p.builds = []int{steps - 1}
	case p.media == postBuilds && builds == "":
		for b := range steps {
			p.builds = append(p.builds, b)
		}
	}
	if builds == "" {
		return p, nil
	}
	for part := range strings.SplitSeq(builds, ",") {
		lo, hi, isRange := strings.Cut(part, "-")
		if !isRange {
			hi = lo
		}
		a, errA := strconv.Atoi(lo)
		b, errB := strconv.Atoi(hi)
		switch {
		case errA != nil || errB != nil || a < 1 || b < a:
			return p, fmt.Errorf("deck: Notes line %d: builds=%s: want 1-based builds and ranges, like 2-4,6", line, builds)
		case b > steps:
			return p, fmt.Errorf("deck: Notes line %d: build %d, but slide %d has %s",
				line, b, slide+1, plural(steps, "build"))
		}
		for x := a; x <= b; x++ {
			p.builds = append(p.builds, x-1)
		}
	}
	return p, nil
}

// unindent is line without the up to three spaces of indent Markdown allows
// before a heading or a fence; false if it is indented code.
func unindent(line string) (string, bool) {
	s := strings.TrimLeft(line, " ")
	return s, len(line)-len(s) <= 3
}

// noteHeading returns the text of line if it is a level-two ATX heading,
// without its optional closing run of #s. "## C#" keeps its #: a closing
// run needs a space before it.
func noteHeading(line string) (string, bool) {
	s, ok := unindent(line)
	if !ok {
		return "", false
	}
	rest, ok := strings.CutPrefix(s, "##")
	if !ok || rest != "" && rest[0] != ' ' && rest[0] != '\t' {
		return "", false
	}
	rest = strings.TrimSpace(rest)
	if t := strings.TrimRight(rest, "#"); t == "" || strings.HasSuffix(t, " ") || strings.HasSuffix(t, "\t") {
		rest = strings.TrimSpace(t)
	}
	return rest, true
}

// openFence reports whether line opens a code fence: a run of three or more
// backticks or tildes, then the info string.
func openFence(line string) (marker, info string, ok bool) {
	s, ok := unindent(line)
	if !ok || s == "" || s[0] != '`' && s[0] != '~' {
		return "", "", false
	}
	n := len(s) - len(strings.TrimLeft(s, s[:1]))
	// A backtick fence's info string can't hold a backtick: that line is
	// inline code.
	if n < 3 || s[0] == '`' && strings.Contains(s[n:], "`") {
		return "", "", false
	}
	return s[:n], strings.TrimSpace(s[n:]), true
}

// nextFence is the code fence open after line, given the one open before
// it: a run of the same character at least as long, alone on its line,
// closes it.
func nextFence(open, line string) string {
	s, ok := unindent(line)
	if !ok || s == "" || s[0] != open[0] {
		return open
	}
	n := len(s) - len(strings.TrimLeft(s, s[:1]))
	if n >= len(open) && strings.TrimSpace(s[n:]) == "" {
		return ""
	}
	return open
}

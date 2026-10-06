package decker

import "strings"

// ParseSpans turns lightly marked-up text into spans styled from theme t, for
// Rich. The markup is small on purpose:
//
//	*bold*        the theme's Display face, since a theme has no bold cut
//	_muted_       t.Muted
//	`code`        t.Mono on a plate; nothing inside is markup
//	{accent:word} a color tag: text, muted, faint, accent, accent2, warn,
//	              good, or "#rrggbb" / "#rgb"
//	{u:word}      underline, in the span's color
//	{s:word}      strikethrough
//	{mark:word}   a highlighter pen: Accent plate, Background ink
//	\*            a backslash makes the next marker character literal; the
//	              markers are * _ ` { } and \ itself
//
// Markers toggle wherever they appear, so "*bo*ld" is bold inside a
// word; write snake\_case with a backslash. Tags nest ({accent:a {u:b}}), and
// the innermost style wins. Inside code, only \` is an escape. A "{" that
// does not start a known tag is literal along with the "}" that matches it,
// so braces nest inside a tag ({accent:map{k}}); a stray "}" is literal too.
// A marker never closed styles to the end. Text is otherwise kept as written,
// including "\n".
func ParseSpans(s string, t *Theme) []Span {
	var (
		out               []Span
		buf               strings.Builder
		bold, muted, code bool
		tags              []string
	)
	flush := func() {
		if buf.Len() == 0 {
			return
		}
		sp := Span{Text: buf.String()}
		buf.Reset()
		if bold {
			sp.Font = t.Display
		}
		if muted {
			c := t.Muted
			sp.Color = &c
		}
		if code {
			m := Mix(t.Panel, t.Faint, 0.5)
			sp.Font, sp.Mark = t.Mono, &m
		}
		for _, tag := range tags {
			if tag != literalBrace {
				applyTag(&sp, tag, t)
			}
		}
		out = append(out, sp)
	}

	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case code && c == '\\' && i+1 < len(s) && s[i+1] == '`':
			buf.WriteByte('`')
			i += 2
		case code && c == '`':
			flush()
			code = false
			i++
		case code:
			buf.WriteByte(c)
			i++
		case c == '\\' && i+1 < len(s) && strings.IndexByte("*_`{}\\", s[i+1]) >= 0:
			buf.WriteByte(s[i+1])
			i += 2
		case c == '*':
			flush()
			bold = !bold
			i++
		case c == '_':
			flush()
			muted = !muted
			i++
		case c == '`':
			flush()
			code = true
			i++
		case c == '{':
			if name, n := tagAt(s[i:], t); n > 0 {
				flush()
				tags = append(tags, name)
				i += n
				break
			}
			// A literal "{" owns the next "}", so that one cannot close a
			// tag opened around it.
			tags = append(tags, literalBrace)
			buf.WriteByte(c)
			i++
		case c == '}' && len(tags) > 0:
			if tags[len(tags)-1] == literalBrace {
				buf.WriteByte(c)
			} else {
				flush()
			}
			tags = tags[:len(tags)-1]
			i++
		default:
			buf.WriteByte(c)
			i++
		}
	}
	flush()
	return out
}

// literalBrace marks a "{" that is plain text on the tag stack. Tag names
// are never empty, so it cannot collide with one.
const literalBrace = ""

// tagAt reports the name of a "{name:" tag at the start of s and its length,
// or 0 if s does not start with one that applyTag knows.
func tagAt(s string, t *Theme) (name string, n int) {
	end := strings.IndexByte(s, ':')
	if end < 2 {
		return "", 0
	}
	name = s[1:end]
	var probe Span
	if !applyTag(&probe, name, t) {
		return "", 0
	}
	return name, end + 1
}

// applyTag styles sp as the tag name says, and reports whether it knows it.
func applyTag(sp *Span, name string, t *Theme) bool {
	color := func(c RGB) bool { sp.Color = &c; return true }
	switch name {
	case "text":
		return color(t.Text)
	case "muted":
		return color(t.Muted)
	case "faint":
		return color(t.Faint)
	case "accent":
		return color(t.Accent)
	case "accent2":
		return color(t.Accent2)
	case "warn":
		return color(t.Warn)
	case "good":
		return color(t.Good)
	case "u":
		sp.Underline = true
		return true
	case "s":
		sp.Strike = true
		return true
	case "mark":
		m := t.Accent
		sp.Mark = &m
		return color(t.Background)
	}
	if isHexColor(name) {
		return color(Hex(name))
	}
	return false
}

func isHexColor(s string) bool {
	if len(s) != 4 && len(s) != 7 || s[0] != '#' {
		return false
	}
	for _, c := range s[1:] {
		if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return false
		}
	}
	return true
}

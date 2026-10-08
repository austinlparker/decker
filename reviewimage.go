package decker

import (
	"fmt"
	"image"
	"strings"
	"sync"
)

// The review images are read by people and by agents that look at pictures,
// so they are drawn at a fixed scale in fixed colors, whatever the deck's
// theme: a frame, outlines around what it drew, numbered boxes around its
// issues, and the issues listed under it.
var (
	reviewInk     = Hex("#E8ECF2")
	reviewMuted   = Hex("#9AA4B2")
	reviewPlate   = Hex("#14181F")
	reviewOutline = Hex("#5FB7FF")
	reviewColors  = map[Severity]RGB{
		SeverityError:   Hex("#FF4D5E"),
		SeverityWarning: Hex("#FFB224"),
		SeverityInfo:    Hex("#7C8CFF"),
	}
	reviewFont = sync.OnceValue(func() *Font { return StockFont("SpaceGrotesk-Medium") })
)

const (
	reviewText  = 15 // annotation text size, in image pixels
	reviewPad   = 12
	reviewWidth = 900 // frames are scaled up by whole pixels to about this wide
)

// reviewScale is the whole-pixel scale that brings a w-pixel-wide frame to
// about reviewWidth.
func reviewScale(w int) int { return max(1, (reviewWidth+w/2)/w) }

// annotate draws f with its elements outlined and its issues boxed and
// listed under it, with a caption naming the slide, build and size.
func annotate(f *reviewedFrame, title string) *image.RGBA {
	src := f.sc.Px
	s := reviewScale(src.W)
	fw, fh := src.W*s, src.H*s
	font := reviewFont()
	lineH := float64(reviewText) * 1.35
	textW := float64(fw)

	// The issue list's height depends on how its lines wrap.
	var notes []string
	for n, is := range f.issues {
		notes = append(notes, fmt.Sprintf("%d  %s  %s: %s", n+1, is.Severity, is.Code, is.Msg))
	}
	listH := 0.0
	for _, note := range notes {
		listH += float64(len(font.Wrap(note, reviewText, textW))) * lineH
	}
	if len(notes) > 0 {
		listH += reviewPad
	}
	headH := lineH + reviewPad
	W, H := fw+2*reviewPad, int(headH+float64(fh)+listH)+2*reviewPad
	p := NewPixels(W, H, reviewPlate)

	ox, oy := float64(reviewPad), float64(reviewPad)+headH
	caption := fmt.Sprintf("%d.%d  %s  ·  %dx%d", f.slide+1, f.step+1, title, f.w, f.h)
	Text{Font: font, Size: reviewText, Color: reviewInk}.Draw(p, caption, ox, float64(reviewPad))
	if counts := issueCounts(f.issues); counts != "" {
		Text{Font: font, Size: reviewText, Color: reviewMuted, Align: Right}.Draw(p, counts, ox+float64(fw), float64(reviewPad))
	}

	// The frame, scaled up by whole pixels so its pixels stay crisp.
	big := make([]RGB, fw*fh)
	boxScale(big, fw, fh, src.Pix, src.W, src.H)
	for y := range fh {
		copy(p.Pix[(int(oy)+y)*W+int(ox):], big[y*fw:(y+1)*fw])
	}
	// The canvas edge, so ink cut off by it reads as cut off.
	p.RoundRect(ox-2, oy-2, float64(fw)+4, float64(fh)+4, 0, 1, reviewMuted, 0.5)

	at := func(r Rect) Rect {
		return Rect{ox + r.X*float64(s), oy + r.Y*float64(s), r.W * float64(s), r.H * float64(s)}
	}
	for _, e := range f.elems {
		r := at(e.r)
		p.RoundRect(r.X, r.Y, r.W, r.H, 0, 1, reviewOutline, 0.45)
	}
	for n, is := range f.issues {
		col := reviewColors[is.Severity]
		if is.Rect == (Rect{}) {
			continue
		}
		r := at(is.Rect)
		p.RoundRect(r.X, r.Y, r.W, r.H, 3, 2.5, col, 1)
		// The badge sits on the box's top-left corner, kept on the image.
		bx := min(max(r.X, ox+9), ox+float64(fw)-9)
		by := min(max(r.Y, oy+9), oy+float64(fh)-9)
		p.Disc(bx, by, 9, col, 1)
		Text{Font: font, Size: 13, Color: reviewPlate, Align: Center}.DrawMid(p, fmt.Sprint(n+1), bx, by)
	}

	y := oy + float64(fh) + reviewPad
	for n, note := range notes {
		_, h := Text{Font: font, Size: reviewText, Color: reviewColors[f.issues[n].Severity], MaxW: textW, Leading: 1.35}.Draw(p, note, ox, y)
		y += h
	}
	return pixelsImage(p)
}

// issueCounts summarizes issues by severity: "1 error, 2 warnings".
func issueCounts(issues []Issue) string {
	var n [3]int
	for _, is := range issues {
		n[is.Severity]++
	}
	var parts []string
	for _, c := range []struct {
		sev       Severity
		one, many string
	}{{SeverityError, "error", "errors"}, {SeverityWarning, "warning", "warnings"}, {SeverityInfo, "note", "notes"}} {
		if n[c.sev] > 0 {
			parts = append(parts, plural(n[c.sev], c.one, c.many))
		}
	}
	return strings.Join(parts, ", ")
}

// worst is the most severe of issues, or -1 for none.
func worst(issues []Issue) Severity {
	w := Severity(-1)
	for _, is := range issues {
		w = max(w, is.Severity)
	}
	return w
}

// sheetTile is one build on a review contact sheet.
type sheetTile struct {
	label string
	worst Severity
	pix   []RGB
	w, h  int
}

// tile shrinks f's frame for a contact sheet: about 320 pixels wide, but
// never enlarged and never by a fraction.
func tile(f *reviewedFrame, title string) sheetTile {
	src := f.sc.Px
	k := max(1, src.W/320)
	t := sheetTile{label: fmt.Sprintf("%d.%d %s", f.slide+1, f.step+1, title), worst: worst(f.issues), w: src.W / k, h: src.H / k}
	t.pix = make([]RGB, t.w*t.h)
	boxScale(t.pix, t.w, t.h, src.Pix, src.W, src.H)
	return t
}

// reviewSheet lays tiles out four across, each under its label and framed in
// the color of its worst issue.
func reviewSheet(tiles []sheetTile, caption string) *image.RGBA {
	const cols, gap, border = 4, 14, 3
	font := reviewFont()
	tw, th := tiles[0].w, tiles[0].h
	const labelH = reviewText*4/3 + 4
	cellW, cellH := tw+2*border, labelH+th+2*border
	rows := (len(tiles) + cols - 1) / cols
	const headH = reviewText*4/3 + gap
	W := cols*cellW + (cols+1)*gap
	H := headH + rows*cellH + (rows+1)*gap
	p := NewPixels(W, H, reviewPlate)
	Text{Font: font, Size: reviewText, Color: reviewInk}.Draw(p, caption, gap, gap)
	for i, t := range tiles {
		x := gap + (i%cols)*(cellW+gap)
		y := headH + gap + (i/cols)*(cellH+gap)
		col, ok := reviewColors[t.worst]
		if !ok {
			col = Hex("#2A303A")
		}
		label := t.label
		if lines := font.Wrap(label, reviewText-2, float64(cellW)); len(lines) > 0 && lines[0] != label {
			label = lines[0] + "…"
		}
		Text{Font: font, Size: reviewText - 2, Color: reviewMuted}.Draw(p, label, float64(x), float64(y))
		fy := y + labelH
		p.Rect(float64(x), float64(fy), float64(cellW), float64(th+2*border), col, 1)
		for ty := range t.h {
			copy(p.Pix[(fy+border+ty)*W+x+border:], t.pix[ty*t.w:(ty+1)*t.w])
		}
	}
	return pixelsImage(p)
}

// pixelsImage converts a canvas to an image, for PNG.
func pixelsImage(p *Pixels) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, p.W, p.H))
	for i, c := range p.Pix {
		q := c.q()
		img.Pix[4*i], img.Pix[4*i+1], img.Pix[4*i+2], img.Pix[4*i+3] = q[0], q[1], q[2], 255
	}
	return img
}

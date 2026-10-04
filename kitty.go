package decker

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"os"
	"strings"
)

// Sharp slide previews for the presenter view, using the kitty graphics
// protocol (Ghostty and kitty support it). A preview is a real image of the
// slide at the deck's full resolution. It's placed with Unicode
// placeholders: cells holding U+10EEEE, whose foreground color names the
// image and whose combining marks give the row and column. The terminal
// draws the matching piece of the image in each cell, so the images are
// ordinary text to Bubble Tea's renderer and need no cursor tricks.

// kittyPlaceholder marks a cell that shows part of an image.
const kittyPlaceholder = '\U0010EEEE'

// imagePreviews reports whether to show previews as images, for the
// -previews flag: "image", "cells", or "auto" (images in Ghostty or kitty,
// unless inside tmux or zellij, which don't pass the images through).
func imagePreviews(mode string) bool {
	switch mode {
	case "image":
		return true
	case "cells":
		return false
	}
	if os.Getenv("TMUX") != "" || os.Getenv("ZELLIJ") != "" {
		return false
	}
	return os.Getenv("TERM_PROGRAM") == "ghostty" || os.Getenv("TERM") == "xterm-ghostty" ||
		os.Getenv("TERM") == "xterm-kitty" || os.Getenv("KITTY_WINDOW_ID") != ""
}

// slideImage draws a slide, settled at the given step, at the deck's size:
// one image pixel per canvas pixel, exactly what the projector shows.
func slideImage(s Slide, step, dw, dh int, t *Theme) *image.RGBA {
	frame := renderSlide(s, Ctx{W: dw, H: dh, T: Settled, Step: step, StepT: Settled, Theme: t})
	px := framePixels(frame, dw, dh, t)
	img := image.NewRGBA(image.Rect(0, 0, px.W, px.H))
	for i, c := range px.Pix {
		q := c.q()
		img.Pix[4*i], img.Pix[4*i+1], img.Pix[4*i+2], img.Pix[4*i+3] = q[0], q[1], q[2], 255
	}
	return img
}

// kittyTransmit returns the escape sequences that upload img as image id
// and make it displayable in a cols×rows area of placeholders. q=2 keeps
// the terminal from replying, which would otherwise arrive as keypresses.
func kittyTransmit(id int, img image.Image, cols, rows int) (string, error) {
	var buf bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&buf, img); err != nil {
		return "", err
	}
	data := base64.StdEncoding.EncodeToString(buf.Bytes())
	const chunk = 4096 // the protocol's maximum payload per sequence
	var b strings.Builder
	for i := 0; i < len(data); i += chunk {
		end := min(i+chunk, len(data))
		more := 0
		if end < len(data) {
			more = 1
		}
		if i == 0 {
			fmt.Fprintf(&b, "\x1b_Ga=T,U=1,f=100,i=%d,c=%d,r=%d,q=2,m=%d;", id, cols, rows, more)
		} else {
			fmt.Fprintf(&b, "\x1b_Gm=%d;", more)
		}
		b.WriteString(data[i:end])
		b.WriteString("\x1b\\")
	}
	return b.String(), nil
}

// kittyDelete returns the sequence that frees image id.
func kittyDelete(id int) string { return fmt.Sprintf("\x1b_Ga=d,d=I,i=%d,q=2\x1b\\", id) }

// kittyPlaceholders returns cols×rows cells that show image id. The id is
// the 256-color foreground, which survives Bubble Tea's color handling
// unchanged; that's why ids stay between 1 and 255.
func kittyPlaceholders(id, cols, rows int) string {
	cols = min(cols, len(kittyDiacritics))
	rows = min(rows, len(kittyDiacritics))
	var b strings.Builder
	for r := 0; r < rows; r++ {
		if r > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "\x1b[38;5;%dm", id)
		for c := 0; c < cols; c++ {
			b.WriteRune(kittyPlaceholder)
			b.WriteRune(kittyDiacritics[r])
			b.WriteRune(kittyDiacritics[c])
		}
		b.WriteString("\x1b[m")
	}
	return b.String()
}

// kittyImages tracks which previews have been uploaded to the terminal.
type kittyImages struct {
	ids  map[previewKey]int
	keys [256]previewKey // by id, to reuse an id's slot
	used [256]bool
	next int // the next id to hand out
	gen  int // bumped by clear; uploads started before it are stale
}

// kittyUploadMsg is a finished preview upload. seq is empty if encoding
// failed.
type kittyUploadMsg struct {
	gen int
	key previewKey
	seq string
}

// finish handles a finished upload: the sequence to send the terminal, or
// "" if the upload is stale (the images were cleared since it started) or
// failed, in which case the key is forgotten so it's tried again.
func (ki *kittyImages) finish(m kittyUploadMsg) string {
	if _, ok := ki.ids[m.key]; m.gen != ki.gen || !ok {
		return "" // cleared, or its id was handed to another preview since
	}
	if m.seq == "" {
		if id, ok := ki.ids[m.key]; ok {
			delete(ki.ids, m.key)
			ki.used[id] = false
		}
		return ""
	}
	return m.seq
}

func newKittyImages() *kittyImages { return &kittyImages{ids: map[previewKey]int{}, next: 1} }

// id returns the image id for k, and whether it's already uploaded.
func (ki *kittyImages) id(k previewKey) (int, bool) {
	id, ok := ki.ids[k]
	return id, ok
}

// add assigns an id to k. If every id is taken, the oldest is reused, and
// the sequence to free it comes back as well.
func (ki *kittyImages) add(k previewKey) (id int, free string) {
	id = ki.next
	ki.next = ki.next%255 + 1
	if ki.used[id] {
		delete(ki.ids, ki.keys[id])
		free = kittyDelete(id)
	}
	ki.ids[k], ki.keys[id], ki.used[id] = id, k, true
	return id, free
}

// clear frees every uploaded image.
func (ki *kittyImages) clear() string {
	var b strings.Builder
	for id, used := range ki.used {
		if used {
			b.WriteString(kittyDelete(id))
		}
	}
	gen := ki.gen + 1
	*ki = *newKittyImages()
	ki.gen = gen
	return b.String()
}

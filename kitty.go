package decker

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
)

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

// kittyTransmit returns the escape sequences that upload img as image id,
// displayable in a cols×rows area of placeholders, or "" if encoding fails.
// q=2 stops the terminal replying, which would arrive as keypresses.
func kittyTransmit(id int, img image.Image, cols, rows int) string {
	var buf bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&buf, img); err != nil {
		return ""
	}
	data := base64.StdEncoding.AppendEncode(nil, buf.Bytes())
	opts := []string{"a=T", "U=1", "f=100", fmt.Sprint("i=", id), fmt.Sprint("c=", cols), fmt.Sprint("r=", rows), "q=2"}
	var b strings.Builder
	for len(data) > 0 {
		n := min(len(data), kitty.MaxChunkSize)
		more := "m=0"
		if n < len(data) {
			more = "m=1"
		}
		b.WriteString(ansi.KittyGraphics(data[:n], append(opts, more)...))
		data, opts = data[n:], nil
	}
	return b.String()
}

func kittyDelete(id int) string {
	return ansi.KittyGraphics(nil, "a=d", "d=I", fmt.Sprint("i=", id), "q=2")
}

// kittyMaxCells is how many row or column numbers kitty defines marks for.
const kittyMaxCells = 297

// kittyPlaceholders returns cols×rows cells showing image id via kitty's
// Unicode placeholders: each cell is U+10EEEE plus marks for its row and
// column, and the terminal draws the matching piece of the image, so images are
// ordinary text to Bubble Tea. The id rides in the 256-color foreground, which
// survives Bubble Tea's color handling; hence ids 1 to 255.
func kittyPlaceholders(id, cols, rows int) string {
	cols, rows = min(cols, kittyMaxCells), min(rows, kittyMaxCells)
	var b strings.Builder
	for r := 0; r < rows; r++ {
		if r > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "\x1b[38;5;%dm", id)
		for c := 0; c < cols; c++ {
			b.WriteRune(kitty.Placeholder)
			b.WriteRune(kitty.Diacritic(r))
			b.WriteRune(kitty.Diacritic(c))
		}
		b.WriteString("\x1b[m")
	}
	return b.String()
}

type kittyImages struct {
	ids  map[previewKey]int
	byID map[int]previewKey // to reuse an id's slot
	next int                // the next id to hand out
	gen  int                // bumped by clear; uploads started before it are stale
}

func newKittyImages() *kittyImages {
	return &kittyImages{ids: map[previewKey]int{}, byID: map[int]previewKey{}, next: 1}
}

// kittyUploadMsg is a finished preview upload; seq is empty if encoding failed.
type kittyUploadMsg struct {
	gen int
	key previewKey
	seq string
}

// finish returns the sequence to send for a finished upload, or "" if it is
// stale (cleared since) or failed; a failed key is forgotten so it's retried.
func (ki *kittyImages) finish(m kittyUploadMsg) string {
	id, ok := ki.ids[m.key]
	if m.gen != ki.gen || !ok {
		return "" // cleared, or its id was handed to another preview since
	}
	if m.seq == "" {
		delete(ki.ids, m.key)
		delete(ki.byID, id)
	}
	return m.seq
}

func (ki *kittyImages) id(k previewKey) (int, bool) {
	id, ok := ki.ids[k]
	return id, ok
}

// add assigns an id to k. If every id is taken, the oldest is reused, and
// the sequence to free it comes back as well.
func (ki *kittyImages) add(k previewKey) (id int, free string) {
	id = ki.next
	ki.next = ki.next%255 + 1
	if old, ok := ki.byID[id]; ok {
		delete(ki.ids, old)
		free = kittyDelete(id)
	}
	ki.ids[k], ki.byID[id] = id, k
	return id, free
}

func (ki *kittyImages) clear() string {
	var b strings.Builder
	for _, id := range slices.Sorted(maps.Keys(ki.byID)) {
		b.WriteString(kittyDelete(id))
	}
	gen := ki.gen + 1
	*ki = *newKittyImages()
	ki.gen = gen
	return b.String()
}

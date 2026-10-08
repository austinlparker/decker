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
// --previews flag: "image", "cells", or "auto" (images in Ghostty or kitty,
// unless inside tmux or zellij, which don't pass the images through).
func imagePreviews(mode string) (bool, error) {
	switch mode {
	case "image":
		return true, nil
	case "cells":
		return false, nil
	case "auto":
	default:
		return false, fmt.Errorf("--previews: want auto, image, or cells")
	}
	if os.Getenv("TMUX") != "" || os.Getenv("ZELLIJ") != "" {
		return false, nil
	}
	return os.Getenv("TERM_PROGRAM") == "ghostty" || os.Getenv("TERM") == "xterm-ghostty" ||
		os.Getenv("TERM") == "xterm-kitty" || os.Getenv("KITTY_WINDOW_ID") != "", nil
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
// ordinary text to Bubble Tea. The id rides in the RGB foreground so a custom
// terminal palette cannot change which image a placeholder refers to.
func kittyPlaceholders(id, cols, rows int) string {
	cols, rows = min(cols, kittyMaxCells), min(rows, kittyMaxCells)
	var b strings.Builder
	for r := 0; r < rows; r++ {
		if r > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "\x1b[38;2;%d;%d;%dm", id>>16&255, id>>8&255, id&255)
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
	ids   map[previewKey]int
	byID  map[int]previewKey
	next  int // the next id to hand out
	gen   int // bumped by clear; uploads started before it are stale
	ready map[previewKey]bool
}

func newKittyImages() *kittyImages {
	return &kittyImages{ids: map[previewKey]int{}, byID: map[int]previewKey{}, ready: map[previewKey]bool{}, next: 1}
}

// kittyUploadMsg is a finished preview upload; seq is empty if encoding failed.
type kittyUploadMsg struct {
	gen int
	key previewKey
	id  int
	seq string
}

func (m kittyUploadMsg) String() string { return m.seq }

func (ki *kittyImages) valid(m kittyUploadMsg) bool {
	id, ok := ki.ids[m.key]
	return m.gen == ki.gen && ok && id == m.id
}

// finish returns the sequence to send for a finished upload, or "" if it is
// stale (cleared since) or failed; a failed key is forgotten so it's retried.
func (ki *kittyImages) finish(m kittyUploadMsg) string {
	if !ki.valid(m) {
		return "" // cleared, or its id was handed to another preview since
	}
	if m.seq == "" {
		delete(ki.ids, m.key)
		delete(ki.byID, m.id)
		delete(ki.ready, m.key)
	}
	return m.seq
}

func (ki *kittyImages) id(k previewKey) (int, bool) {
	id, ok := ki.ids[k]
	return id, ok
}

// add retains at most 255 images. IDs increase even across resizes so a
// queued deletion cannot delete an image uploaded for a newer layout.
func (ki *kittyImages) add(k previewKey) (id int, free string) {
	if ki.next > 0xffffff {
		free = ki.clear()
		ki.next = 1
	}
	id = ki.next
	ki.next++
	if len(ki.byID) >= 255 {
		oldID := slices.Min(slices.Collect(maps.Keys(ki.byID)))
		old := ki.byID[oldID]
		delete(ki.ids, old)
		delete(ki.ready, old)
		delete(ki.byID, oldID)
		free += kittyDelete(oldID)
	}
	ki.ids[k], ki.byID[id] = id, k
	return id, free
}

func (ki *kittyImages) clear() string {
	var b strings.Builder
	for _, id := range slices.Sorted(maps.Keys(ki.byID)) {
		b.WriteString(kittyDelete(id))
	}
	gen, next := ki.gen+1, ki.next
	*ki = *newKittyImages()
	ki.gen, ki.next = gen, next
	return b.String()
}

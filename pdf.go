package decker

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"strings"
	"unicode/utf16"
)

// writePDF writes the deck to path as a PDF (the pdf command).
func writePDF(d *Deck, path string, size pixelSize, builds bool) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := renderPDF(d, f, size, builds); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// renderPDF writes the deck to w as a PDF: a page for each slide at its last
// build, or for every build with builds, each the slide settled on a
// size-pixel canvas, as video draws it, so without the character layer. The
// outline has an entry for each slide, at its first page. Nothing else goes
// in, no date either, so the file depends only on the deck and the flags.
func renderPDF(d *Deck, w io.Writer, size pixelSize, builds bool) error {
	pw := &pdfWriter{w: bufio.NewWriter(w)}
	pw.printf("%%PDF-1.4\n%%\xe2\xe3\xcf\xd3\n") // the comment says the file is binary
	catalog, pages, info, outlines := pw.alloc(), pw.alloc(), pw.alloc(), pw.alloc()

	img := image.NewRGBA(image.Rect(0, 0, size.W, size.H))
	var kids []int
	firstPage := make([]int, len(d.Slides))
	for i, s := range d.Slides {
		first := s.steps() - 1
		if builds {
			first = 0
		}
		for step := first; step < s.steps(); step++ {
			sc := d.scene(i, stillCtx(size.W, size.H/2, step, Settled))
			pasteImage(img, 0, 0, sc.Px.Pix, sc.Px.W)
			sc.Release()
			data, err := flateRGB(img)
			if err != nil {
				return fmt.Errorf("pdf: slide %d: %w", i+1, err)
			}
			page, content, im := pw.alloc(), pw.alloc(), pw.alloc()
			pw.object(page, fmt.Sprintf("<< /Type /Page /Parent %d 0 R /MediaBox [0 0 %d %d] "+
				"/Resources << /XObject << /Im %d 0 R >> >> /Contents %d 0 R >>", pages, size.W, size.H, im, content), nil)
			pw.object(content, "<<", fmt.Appendf(nil, "q %d 0 0 %d 0 0 cm /Im Do Q", size.W, size.H))
			pw.object(im, fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width %d /Height %d "+
				"/ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /FlateDecode "+
				"/DecodeParms << /Predictor 15 /Colors 3 /BitsPerComponent 8 /Columns %d >>", size.W, size.H, size.W), data)
			kids = append(kids, page)
			if step == first {
				firstPage[i] = page
			}
		}
	}

	items := make([]int, len(d.Slides))
	for i := range items {
		items[i] = pw.alloc()
	}
	for i, s := range d.Slides {
		title := s.Title
		if title == "" {
			title = fmt.Sprintf("Slide %d", i+1)
		}
		dict := fmt.Sprintf("<< /Title %s /Parent %d 0 R /Dest [%d 0 R /Fit]", pdfText(title), outlines, firstPage[i])
		if i > 0 {
			dict += fmt.Sprintf(" /Prev %d 0 R", items[i-1])
		}
		if i < len(items)-1 {
			dict += fmt.Sprintf(" /Next %d 0 R", items[i+1])
		}
		pw.object(items[i], dict+" >>", nil)
	}
	pw.object(outlines, fmt.Sprintf("<< /Type /Outlines /First %d 0 R /Last %d 0 R /Count %d >>",
		items[0], items[len(items)-1], len(items)), nil)

	refs := make([]string, len(kids))
	for i, k := range kids {
		refs[i] = fmt.Sprintf("%d 0 R", k)
	}
	pw.object(pages, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(refs, " "), len(kids)), nil)
	pw.object(info, fmt.Sprintf("<< /Title %s /Creator (decker) >>", pdfText(d.Name)), nil)
	pw.object(catalog, fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R /Outlines %d 0 R >>", pages, outlines), nil)
	return pw.finish(catalog, info)
}

// pdfWriter writes a PDF's objects as they come, keeping each one's offset
// for the cross-reference table. The first error sticks; finish returns it.
type pdfWriter struct {
	w    *bufio.Writer
	n    int
	offs []int // of object i+1; objects are numbered from 1
	err  error
}

// Write counts what it writes, and does nothing after an error.
func (p *pdfWriter) Write(b []byte) (int, error) {
	if p.err != nil {
		return 0, p.err
	}
	n, err := p.w.Write(b)
	p.n += n
	p.err = err
	return n, err
}

func (p *pdfWriter) printf(format string, a ...any) { fmt.Fprintf(p, format, a...) }

// alloc numbers an object, to refer to before it's written.
func (p *pdfWriter) alloc() int {
	p.offs = append(p.offs, 0)
	return len(p.offs)
}

// object writes object num: dict alone, or with stream, dict without its
// closing >>, to which object adds the stream's /Length.
func (p *pdfWriter) object(num int, dict string, stream []byte) {
	p.offs[num-1] = p.n
	p.printf("%d 0 obj\n%s", num, dict)
	if stream != nil {
		p.printf(" /Length %d >>\nstream\n", len(stream))
		p.Write(stream)
		p.printf("\nendstream")
	}
	p.printf("\nendobj\n")
}

// finish writes the cross-reference table and the trailer, once every
// allocated object is written.
func (p *pdfWriter) finish(root, info int) error {
	xref := p.n
	p.printf("xref\n0 %d\n0000000000 65535 f \n", len(p.offs)+1)
	for _, off := range p.offs {
		p.printf("%010d 00000 n \n", off)
	}
	p.printf("trailer\n<< /Size %d /Root %d 0 R /Info %d 0 R >>\nstartxref\n%d\n%%%%EOF\n",
		len(p.offs)+1, root, info, xref)
	if p.err != nil {
		return p.err
	}
	return p.w.Flush()
}

// flateRGB is img's colors as a PDF image stream: FlateDecode with PNG
// predictors, which is what an 8-bit truecolor PNG's IDAT chunks hold, so
// image/png does the filtering and the compressing.
func flateRGB(img *image.RGBA) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	var out []byte
	for b := buf.Bytes()[len("\x89PNG\r\n\x1a\n"):]; len(b) >= 12; {
		n := binary.BigEndian.Uint32(b)
		typ, data := string(b[4:8]), b[8:8+n]
		switch typ {
		case "IHDR":
			// image/png writes an opaque image as truecolor, never
			// interlaced, but its docs don't promise it.
			if data[9] != 2 || data[12] != 0 {
				return nil, errors.New("image/png wrote other than 8-bit truecolor")
			}
		case "IDAT":
			out = append(out, data...)
		}
		b = b[12+n:]
	}
	return out, nil
}

// pdfText is s as a PDF text string: UTF-16BE with a byte-order mark, in hex,
// so nothing in it needs escaping.
func pdfText(s string) string {
	var b strings.Builder
	b.WriteString("<FEFF")
	for _, u := range utf16.Encode([]rune(s)) {
		fmt.Fprintf(&b, "%04X", u)
	}
	return b.String() + ">"
}

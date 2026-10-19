package decker

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode/utf16"
)

// TestPDF reads a PDF back as a viewer would, from its cross-reference
// table: a page for each slide, or each build, holding that build's frame
// exactly, and an outline entry for each slide at its first page.
func TestPDF(t *testing.T) {
	d := testDeck()
	size := pixelSize{96, 54}
	for _, builds := range []bool{false, true} {
		var buf bytes.Buffer
		if err := renderPDF(d, &buf, size, builds); err != nil {
			t.Fatal(err)
		}
		objs := readPDF(t, buf.Bytes())
		root := objs[pdfRef(t, objs[0].dict, "Root")]
		if got := pdfString(t, pdfField(t, objs[pdfRef(t, objs[0].dict, "Info")].dict, "Title")); got != d.Name {
			t.Errorf("title %q, want %q", got, d.Name)
		}

		type build struct{ slide, step int }
		var want []build
		for i, s := range d.Slides {
			for step := range s.steps() {
				if builds || step == s.steps()-1 {
					want = append(want, build{i, step})
				}
			}
		}
		kids := pdfRefs(pdfField(t, objs[pdfRef(t, root.dict, "Pages")].dict, "Kids"))
		if len(kids) != len(want) {
			t.Fatalf("builds %v: %d pages, want %d", builds, len(kids), len(want))
		}
		first := map[int]int{} // a slide's first page's object
		for p, k := range kids {
			b := want[p]
			if _, ok := first[b.slide]; !ok {
				first[b.slide] = k
			}
			page := objs[k].dict
			if mb := pdfField(t, page, "MediaBox"); mb != "[0 0 96 54]" {
				t.Errorf("page %d: MediaBox %s", p+1, mb)
			}
			img, err := png.Decode(bytes.NewReader(asPNG(size, objs[pdfRef(t, page, "Im")].stream)))
			if err != nil {
				t.Fatalf("page %d: %v", p+1, err)
			}
			sc := d.scene(b.slide, stillCtx(size.W, size.H/2, b.step, Settled))
			for j, c := range sc.Px.Pix {
				r, g, bl, _ := img.At(j%size.W, j/size.W).RGBA()
				if q := c.q(); [3]uint8{uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8)} != q {
					t.Errorf("page %d (slide %d, build %d): pixel %d is %v, the frame has %v",
						p+1, b.slide+1, b.step+1, j, [3]uint32{r >> 8, g >> 8, bl >> 8}, q)
					break
				}
			}
			sc.Release()
		}

		item := pdfRef(t, objs[pdfRef(t, root.dict, "Outlines")].dict, "First")
		for i, s := range d.Slides {
			o := objs[item].dict
			if got := pdfString(t, pdfField(t, o, "Title")); got != s.Title {
				t.Errorf("outline entry %d is %q, want %q", i+1, got, s.Title)
			}
			if dest := pdfField(t, o, "Dest"); dest != "["+strconv.Itoa(first[i])+" 0 R /Fit]" {
				t.Errorf("outline entry %d goes to %s, want slide %d's first page, object %d", i+1, dest, i+1, first[i])
			}
			if i < len(d.Slides)-1 {
				item = pdfRef(t, o, "Next")
			} else if strings.Contains(o, "/Next") {
				t.Errorf("the last outline entry has a next: %s", o)
			}
		}

		var again bytes.Buffer
		if err := renderPDF(d, &again, size, builds); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(buf.Bytes(), again.Bytes()) {
			t.Errorf("builds %v: the same deck wrote two different PDFs", builds)
		}
	}
}

func TestPDFCommand(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deck.pdf")
	if err := runCLI(testDeck(), "pdf", path, "--size", "64x36"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	objs := readPDF(t, b)
	pages := objs[pdfRef(t, objs[pdfRef(t, objs[0].dict, "Root")].dict, "Pages")].dict
	if n := pdfField(t, pages, "Count"); n != strconv.Itoa(len(testDeck().Slides)) {
		t.Errorf("%s pages, want one per slide", n)
	}
}

func TestPDFText(t *testing.T) {
	for _, s := range []string{"", "Hello (deck)", `back\slash`, "café 🎤"} {
		if got := pdfString(t, pdfText(s)); got != s {
			t.Errorf("pdfText(%q) reads back as %q", s, got)
		}
	}
}

// pdfObject is an object of a PDF as renderPDF writes it: a dictionary, and
// its stream if it has one. Object 0 stands for the trailer.
type pdfObject struct {
	dict   string
	stream []byte
}

var (
	pdfTail   = regexp.MustCompile(`trailer\n(<<.*>>)\nstartxref\n(\d+)\n%%EOF\n$`)
	pdfStream = regexp.MustCompile(`^(<<.*) /Length (\d+) >>\nstream\n`)
)

// readPDF finds every object of b through its cross-reference table.
func readPDF(t *testing.T, b []byte) map[int]pdfObject {
	t.Helper()
	if !bytes.HasPrefix(b, []byte("%PDF-1.4\n")) {
		t.Fatalf("no PDF header: %q", b[:min(len(b), 16)])
	}
	m := pdfTail.FindSubmatch(b)
	if m == nil {
		t.Fatalf("no trailer: %q", b[max(0, len(b)-120):])
	}
	objs := map[int]pdfObject{0: {dict: string(m[1])}}
	xref, _ := strconv.Atoi(string(m[2]))
	var n int
	head := string(b[xref:min(len(b), xref+32)])
	if _, err := fmt.Sscanf(head, "xref\n0 %d\n", &n); err != nil {
		t.Fatalf("no xref table at %d: %q", xref, head)
	}
	if size := pdfField(t, objs[0].dict, "Size"); size != strconv.Itoa(n) {
		t.Errorf("trailer /Size %s, xref has %d entries", size, n)
	}
	entries := b[xref+len("xref\n0 \n")+len(strconv.Itoa(n)):]
	for i := 1; i < n; i++ {
		e := string(entries[20*i : 20*(i+1)])
		off, err := strconv.Atoi(e[:10])
		if err != nil || e[10:] != " 00000 n \n" {
			t.Fatalf("xref entry %d: %q", i, e)
		}
		body, ok := bytes.CutPrefix(b[off:], []byte(strconv.Itoa(i)+" 0 obj\n"))
		if !ok {
			t.Fatalf("object %d isn't at %d: %q", i, off, b[off:min(len(b), off+20)])
		}
		if m := pdfStream.FindSubmatch(body[:bytes.IndexByte(body, '\n')+len("\nstream\n")]); m != nil {
			length, _ := strconv.Atoi(string(m[2]))
			start := len(m[0])
			stream := body[start : start+length]
			if !bytes.HasPrefix(body[start+length:], []byte("\nendstream\nendobj\n")) {
				t.Fatalf("object %d's stream isn't %d bytes", i, length)
			}
			objs[i] = pdfObject{string(m[1]) + " >>", stream}
			continue
		}
		dict, _, ok := bytes.Cut(body, []byte("\nendobj\n"))
		if !ok {
			t.Fatalf("object %d never ends", i)
		}
		objs[i] = pdfObject{dict: string(dict)}
	}
	return objs
}

// pdfField is the value of /name in dict, up to the next key or the end.
func pdfField(t *testing.T, dict, name string) string {
	t.Helper()
	m := regexp.MustCompile(`/` + name + ` (\[[^]]*\]|<[^<>]*>|\([^)]*\)|[^/>]+?)\s*(/|>>)`).FindStringSubmatch(dict)
	if m == nil {
		t.Fatalf("no /%s in %s", name, dict)
	}
	return m[1]
}

// pdfRef is the object /name refers to in dict.
func pdfRef(t *testing.T, dict, name string) int {
	t.Helper()
	r := pdfRefs(pdfField(t, dict, name))
	if len(r) != 1 {
		t.Fatalf("/%s in %s isn't a reference", name, dict)
	}
	return r[0]
}

func pdfRefs(s string) []int {
	var out []int
	for _, m := range regexp.MustCompile(`(\d+) 0 R`).FindAllStringSubmatch(s, -1) {
		n, _ := strconv.Atoi(m[1])
		out = append(out, n)
	}
	return out
}

// pdfString reads a hex text string as pdfText writes it.
func pdfString(t *testing.T, s string) string {
	t.Helper()
	hex, ok := strings.CutPrefix(s, "<FEFF")
	if !ok || !strings.HasSuffix(hex, ">") || (len(hex)-1)%4 != 0 {
		t.Fatalf("not a UTF-16 hex string: %s", s)
	}
	var u []uint16
	for i := 0; i+4 < len(hex); i += 4 {
		v, err := strconv.ParseUint(hex[i:i+4], 16, 16)
		if err != nil {
			t.Fatalf("%s: %v", s, err)
		}
		u = append(u, uint16(v))
	}
	return string(utf16.Decode(u))
}

// asPNG wraps an image stream in the PNG it came out of.
func asPNG(size pixelSize, idat []byte) []byte {
	var b bytes.Buffer
	b.WriteString("\x89PNG\r\n\x1a\n")
	chunk := func(typ string, data []byte) {
		binary.Write(&b, binary.BigEndian, uint32(len(data)))
		b.WriteString(typ)
		b.Write(data)
		binary.Write(&b, binary.BigEndian, crc32.ChecksumIEEE(append([]byte(typ), data...)))
	}
	ihdr := binary.BigEndian.AppendUint32(binary.BigEndian.AppendUint32(nil, uint32(size.W)), uint32(size.H))
	chunk("IHDR", append(ihdr, 8, 2, 0, 0, 0))
	chunk("IDAT", idat)
	chunk("IEND", nil)
	return b.Bytes()
}

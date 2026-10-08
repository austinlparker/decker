package decker

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestHandout(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "out", "handout") // made if missing
	o := options{handout: dir, at: Settled, width: 40, height: 12}
	if err := runHandout(citedDeck(), o); err != nil {
		t.Fatal(err)
	}
	md, err := os.ReadFile(filepath.Join(dir, "handout.md"))
	if err != nil {
		t.Fatal(err)
	}
	const want = `# cited

4 slides, 5 builds.

## 1. Opening

*Intro · 1 build*

![Slide 1](01.png)

Sources:

- [Go (language)](<https://en.wikipedia.org/wiki/Go_(programming_language)>)

## 2. Logs \*and\* traces

*Intro · 2 builds*

![Slide 2](02.png)

Say why logs matter.

Then the model.

Sources:

- [OpenTelemetry Logs Data Model, v1.40](https://opentelemetry.io/docs/specs/otel/logs/data-model/)
- A book, ch. 3

## 3. Again

*Detail · 1 build*

![Slide 3](03.png)

Sources:

- [OTel logs](https://opentelemetry.io/docs/specs/otel/logs/data-model/)
- A book, ch. 3

## 4. Plain

*Detail · 1 build*

![Slide 4](04.png)

## Sources

- [Go (language)](<https://en.wikipedia.org/wiki/Go_(programming_language)>) — slide 1
- [OpenTelemetry Logs Data Model, v1.40](https://opentelemetry.io/docs/specs/otel/logs/data-model/) — slides 2, 3
- A book, ch. 3 — slides 2, 3
`
	if string(md) != want {
		t.Errorf("handout.md:\n%s\nwant:\n%s", md, want)
	}
	for _, name := range []string{"01.png", "02.png", "03.png", "04.png"} {
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := png.DecodeConfig(f)
		f.Close()
		if err != nil || cfg.Width != 40*cellW || cfg.Height != 12*cellH {
			t.Errorf("%s: %dx%d, %v; want a 40x12-cell frame", name, cfg.Width, cfg.Height, err)
		}
	}

	// The same deck gives the same bytes: nothing depends on the clock or
	// on map order.
	again := filepath.Join(t.TempDir(), "again")
	o.handout = again
	if err := runHandout(citedDeck(), o); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"handout.md", "02.png"} {
		a, _ := os.ReadFile(filepath.Join(dir, name))
		b, _ := os.ReadFile(filepath.Join(again, name))
		if string(a) != string(b) {
			t.Errorf("%s differs between runs", name)
		}
	}
}

func TestHandoutWithoutSources(t *testing.T) {
	dir := t.TempDir()
	d := &Deck{Name: "bare", Theme: testTheme, Slides: []Slide{{Title: "Only"}}}
	if err := runHandout(d, options{handout: dir, at: Settled, width: 20, height: 6}); err != nil {
		t.Fatal(err)
	}
	md, _ := os.ReadFile(filepath.Join(dir, "handout.md"))
	const want = "# bare\n\n1 slide, 1 build.\n\n## 1. Only\n\n*1 build*\n\n![Slide 1](01.png)\n"
	if string(md) != want {
		t.Errorf("handout.md = %q, want %q", md, want)
	}
}

func TestHandoutRejects(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for name, o := range map[string]options{
		"zero width":    {handout: t.TempDir(), width: 0, height: 12},
		"negative":      {handout: t.TempDir(), width: 40, height: -1},
		"dir is a file": {handout: file, width: 40, height: 12},
		"under a file":  {handout: filepath.Join(file, "sub"), width: 40, height: 12},
	} {
		if err := runHandout(citedDeck(), o); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestThumbNames(t *testing.T) {
	for _, c := range []struct {
		n           int
		first, last string
	}{{1, "01.png", "01.png"}, {9, "01.png", "09.png"}, {42, "01.png", "42.png"}, {120, "001.png", "120.png"}} {
		names := thumbNames(c.n)
		if len(names) != c.n || names[0] != c.first || names[c.n-1] != c.last {
			t.Errorf("thumbNames(%d) = %s … %s", c.n, names[0], names[len(names)-1])
		}
	}
}

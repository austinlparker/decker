package decker

import (
	"slices"
	"testing"
)

func TestParseDevList(t *testing.T) {
	out := "/talk\tnotes.md\timg/logo.png\timg/bg.png\n\n/talk/slides\n/talk/fonts\tf.ttf\n"
	w := parseDevList(out)
	if want := []string{"/talk", "/talk/img", "/talk/slides", "/talk/fonts"}; !slices.Equal(w.dirs, want) {
		t.Errorf("dirs %q, want %q", w.dirs, want)
	}
	for path, want := range map[string]bool{
		"/talk/notes.md":      true,
		"/talk/img/logo.png":  true,
		"/talk/fonts/f.ttf":   true,
		"/talk/slides/a.go":   true,
		"/talk/go.mod":        true,
		"/talk/README.md":     false,
		"/talk/img/other.png": false,
		"/talk/.notes.md.swp": false,
		"/talk/.#main.go":     false,
		"/talk/slides/a.go~":  false,
	} {
		if got := w.rebuilds(path); got != want {
			t.Errorf("rebuilds(%q) = %v, want %v", path, got, want)
		}
	}
}

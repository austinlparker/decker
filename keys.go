package decker

import (
	"strconv"
	"strings"
)

type binding struct {
	keys string // space-separated key names, as tea.KeyPressMsg.String reports them
	act  string // what the model does: see model.handleKey
	nav  bool   // the presenter view may press these on the deck's behalf
	help string // the help box row, if any
	what string
}

// bindings is the one key table, shared by the deck, the presenter view and
// the help box. To add a key, add it here.
var bindings = []binding{
	{"right l space pgdown j down enter", "next", true, "→ l space pgdn", "next step / slide"},
	{"left h pgup k up backspace", "prev", true, "← h pgup", "previous"},
	{"]", "nextSlide", true, "] [", "next / previous slide (skip steps)"},
	{"[", "prevSlide", true, "", ""},
	{"g home", "first", true, "g G", "first / last slide"},
	{"G end", "last", true, "", ""},
	{"", "", false, "12g  12⏎", "jump to slide 12"},
	{"r", "replay", true, "r", "replay this slide"},
	{"n", "notes", false, "n", "speaker notes"},
	{"?", "help", false, "?", "this help"},
	{"esc", "closeHelp", false, "", ""},
	{"ctrl+l", "redraw", false, "", ""},
	{"q ctrl+c", "quit", false, "q", "quit"},
}

// keyActs maps a key name to its binding.
var keyActs = func() map[string]binding {
	m := map[string]binding{}
	for _, b := range bindings {
		for _, k := range strings.Fields(b.keys) {
			m[k] = b
		}
	}
	return m
}()

// countKey tracks the numeric prefix for jumps ("12g", "12⏎") as keys
// arrive. It reports done when k was a digit, or completed a jump to the
// 1-based slide (0 for a digit).
func countKey(count *string, k string) (slide int, done bool) {
	if len(k) == 1 && k[0] >= '0' && k[0] <= '9' {
		*count += k
		return 0, true
	}
	c := *count
	*count = ""
	if c == "" || (k != "enter" && k != "g" && k != "home") {
		return 0, false
	}
	n, _ := strconv.Atoi(c)
	return max(n, 1), true
}

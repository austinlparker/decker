package decker

// parseGrid draws a styled string into a fresh w×h grid on the theme's
// background.
func parseGrid(s string, w, h int, t *Theme) *grid {
	g := blankGrid(w, h, t.Background)
	g.draw(0, 0, s, false, t.Text)
	return g
}

// composeTransition is composeGrid for frame strings.
func composeTransition(kind Transition, from, to string, w, h int, p float64, forward bool, t *Theme) string {
	if w <= 0 || h <= 0 {
		return to
	}
	a, b := parseGrid(from, w, h, t), parseGrid(to, w, h, t)
	out := composeGrid(kind, a, b, p, forward, t)
	s := out.String()
	a.release()
	b.release()
	out.release()
	return s
}

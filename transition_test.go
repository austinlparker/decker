package decker

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

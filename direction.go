package decker

// Direction is a side of the screen or of an element: where an element or a
// transition's new slide comes from, which edge a wipe starts at, the axis a
// split opens along. DirUp is the top edge and DirDown the bottom.
type Direction int

// The directions.
const (
	// DirDefault is the default side of whatever takes it: each
	// transition's own (see Transition.From), and from below for an element
	// animation, as in PowerPoint.
	DirDefault Direction = iota
	DirLeft
	DirRight
	DirUp
	DirDown
)

package decker

// Direction is a side of the screen or of an element: where an element or a
// transition's new slide comes from, or which edge a wipe starts at.
type Direction int

// The four directions.
const (
	DirLeft Direction = iota
	DirRight
	DirUp
	DirDown
)

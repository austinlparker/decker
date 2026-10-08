package main

// Recipe: an architecture diagram, and a magic move into one service.
//
// What it shows: services as boxes in tiers, left to right (the edge, the
// services behind it, the data they keep), joined by connectors that draw
// on as each tier appears. A second slide zooms into one service: the boxes
// it talks to glide to new places, the rest fade, and the service grows
// into a panel showing what's inside it.
//
// How it's built:
//
//  1. Layout. layoutArchitecture is a pure function of the rect and the
//     nodes: each tier's column is as wide as its longest name (measured
//     with Text.Measure), the gaps between columns as wide as the connector
//     labels need, and the boxes stack in each column with Rect.Rows.
//  2. Check. c.Fits compares that with the rect. When the labels leave the
//     boxes too little room, the layout drops the connector labels rather
//     than shrinking the names below the readable size.
//  3. Draw. Connectors are drawn by the View; the boxes are handed to the
//     engine with sc.Place, under keys. The second slide places the same
//     keys at new rects and enters with TransitionMorph, so each box glides
//     from where it was to where it is now.
//  4. Reveal. Tier t appears on build t: its header and boxes fade in and
//     the connectors into it draw on (Connector.Prog with Ease).
//
// Knobs: the nodes, tiers and links below, the tier colors in tierColor,
// the box height (boxLines), and for the zoom slide, which service to open
// (zoomKey), its parts and its links.

import "github.com/austinlparker/decker"

// node is one box in the diagram.
type node struct {
	key   string // its name on screen, and the key that pairs it across slides
	tier  int    // its column, left to right
	store bool   // drawn as a cylinder: a database, a cache, a queue
}

var archNodes = []node{
	{"gateway", 0, false},
	{"orders", 1, false},
	{"cart", 1, false},
	{"payments", 1, false},
	{"postgres", 2, true},
	{"redis", 2, true},
	{"kafka", 2, true},
}

var archTiers = []string{"edge", "services", "data"}

// link is a connector from one node to another, with an optional label.
type link struct{ from, to, label string }

var archLinks = []link{
	{"gateway", "orders", ""},
	{"gateway", "cart", ""},
	{"gateway", "payments", ""},
	{"orders", "postgres", "SQL"},
	{"cart", "redis", "GET"},
	{"payments", "kafka", "pub"},
}

// boxLines is a box's height in lines of its label's text.
const boxLines = 2.1

func architectureSlide() decker.Slide {
	return decker.Slide{Title: "Architecture", Section: "Architecture", Transition: decker.TransitionPush,
		Steps: len(archTiers), // one build per tier
		Notes: "Edge first, then the services behind it, then where they keep their data.",
		View: func(c decker.Ctx, sc *decker.Scene) {
			p, th := sc.Px, c.Theme
			area := heading(c, p, "Architecture")
			lay := layoutArchitecture(c, area, true)
			if !c.Fits("architecture", area, lay.needW, lay.needH) {
				// The labels need wider gaps than the room allows: drop them
				// and keep the names readable.
				lay = layoutArchitecture(c, area, false)
			}

			// Tier headers, each with its tier, on one baseline.
			head := lay.text
			head.Color, head.Align = th.Muted, decker.Center
			for t, name := range archTiers {
				if !c.Reached(t) {
					continue
				}
				head.FX = decker.FadeUp(c.Since(t), 0.4, head.Size)
				h := lay.heads[t]
				head.Draw(p, name, h.X+h.W/2, h.Y)
			}

			// Connectors draw on with the tier they lead into. The fan-out
			// from the edge curves; links within a row run straight.
			for _, k := range archLinks {
				from, to := nodeIndex(k.from), nodeIndex(k.to)
				route := decker.RouteCurved
				if archNodes[from].tier > 0 {
					route = decker.RouteElbow
				}
				prog := decker.Ease(c.Since(archNodes[to].tier)-0.2, 0.6)
				decker.Connector{From: lay.boxes[from], To: lay.boxes[to], Route: route,
					FromSide: decker.SideRight, ToSide: decker.SideLeft,
					Head: decker.HeadArrow, Color: th.Muted, Prog: prog}.Draw(c, p)
				if lay.labels && k.label != "" {
					linkLabel(c, p, lay.boxes[from], lay.boxes[to], k.label, prog)
				}
			}

			// The boxes, placed by key so the next slide can move them.
			for i, n := range archNodes {
				if !c.Reached(n.tier) {
					continue
				}
				a := decker.Ease(c.Since(n.tier), 0.4)
				sc.Place(n.key, lay.boxes[i], func(p *decker.Pixels, r decker.Rect) {
					drawNode(c, p, r, n, a)
				})
			}
		}}
}

// archLayout is where the diagram goes, worked out before drawing.
type archLayout struct {
	text   decker.Text   // one size for every label
	heads  []decker.Rect // the tier headers
	boxes  []decker.Rect // one per node, in archNodes order
	labels bool          // whether the gaps leave room for connector labels

	needW, needH float64 // the room the layout needs at the least
}

func layoutArchitecture(c decker.Ctx, r decker.Rect, labels bool) archLayout {
	th := c.Theme
	l := archLayout{labels: labels, text: decker.Text{Font: th.Body, Size: c.SmallText(th.Body)}}
	_, lineH := l.text.Measure("Ag")
	pad := float64(l.text.Size) * 0.6

	// Each column is as wide as its widest name, with padding.
	colW := make([]float64, len(archTiers))
	perTier := make([]int, len(archTiers))
	for _, n := range archNodes {
		w, _ := l.text.Measure(n.key)
		colW[n.tier] = max(colW[n.tier], w+2*pad)
		perTier[n.tier]++
	}

	// The gaps leave room for an arrow, and for the widest label above it
	// when there are labels.
	minGap := c.Unit(0.1)
	if labels {
		for _, k := range archLinks {
			minGap = max(minGap, labelGap(c, k.label))
		}
	}
	sum, most := 0.0, 0
	for t := range archTiers {
		sum += colW[t]
		most = max(most, perTier[t])
	}
	boxH := lineH * boxLines
	headH := lineH * 1.3
	gapY := lineH * 0.6
	l.needW = sum + minGap*float64(len(archTiers)-1)
	l.needH = headH + float64(most)*boxH + float64(most-1)*gapY

	// Spare width widens the gaps, up to a point, and then the boxes:
	// Cols shares what's left among the columns by their widths.
	gap := min(max((r.W-sum)/float64(len(archTiers)-1), minGap), r.W*0.16)
	cols := r.Cols(gap, colW...)
	l.heads = make([]decker.Rect, len(archTiers))
	l.boxes = make([]decker.Rect, len(archNodes))
	for t, col := range cols {
		h, rest := col.CutTop(headH)
		l.heads[t] = h
		weights := make([]float64, perTier[t])
		for i := range weights {
			weights[i] = 1
		}
		slots := rest.Rows(gapY, weights...)
		k := 0
		for i, n := range archNodes {
			if n.tier == t {
				l.boxes[i] = slots[k].Anchor(col.W, min(boxH, slots[k].H), 0.5, 0.5)
				k++
			}
		}
	}
	return l
}

// drawNode draws node n in r, faded by a: a rounded box, or a cylinder for
// a store, with its name in the middle. It sizes everything from r, so it
// looks right at every rect a morph passes through.
func drawNode(c decker.Ctx, p *decker.Pixels, r decker.Rect, n node, a float64) {
	th := c.Theme
	edge := tierColor(th, n.tier)
	stroke := max(c.Unit(0.007), 1)
	cy := r.Y + r.H/2
	if n.store {
		cy = drawCylinder(p, r, stroke, th.Panel, edge, a)
	} else {
		rad := min(c.Unit(0.025), r.H/3)
		p.RoundRect(r.X, r.Y, r.W, r.H, rad, 0, th.Panel, a)
		p.RoundRect(r.X, r.Y, r.W, r.H, rad, stroke, edge, a)
	}
	txt := decker.Text{Font: th.Body, Size: c.SmallText(th.Body), Color: decker.Mix(th.Background, th.Text, a), Align: decker.Center}
	txt.DrawMid(p, n.key, r.X+r.W/2, cy)
}

// drawCylinder draws a database-style cylinder filling r and returns the y
// to center a label on: the middle of the body below the top cap.
func drawCylinder(p *decker.Pixels, r decker.Rect, stroke float64, fill, edge decker.RGB, a float64) float64 {
	ry := min(r.H*0.16, r.W*0.12)
	cx := r.X + r.W/2
	top, bot := r.Y+ry, r.Bottom()-ry
	// The bottom cap first, so the body covers its upper half.
	p.Ellipse(cx, bot, r.W/2, ry, 0, fill, a)
	p.Ellipse(cx, bot, r.W/2-stroke/2, ry-stroke/2, stroke, edge, a)
	p.Rect(r.X, top, r.W, bot-top, fill, a)
	p.Rect(r.X, top, stroke, bot-top, edge, a)
	p.Rect(r.Right()-stroke, top, stroke, bot-top, edge, a)
	p.Ellipse(cx, top, r.W/2, ry, 0, decker.Mix(fill, edge, 0.18), a)
	p.Ellipse(cx, top, r.W/2-stroke/2, ry-stroke/2, stroke, edge, a)
	return (top + ry + r.Bottom()) / 2
}

// linkLabel writes a connector's label just above the middle of the run
// between two boxes side by side, fading in as the connector draws on.
// Above the line rather than on it, the label needs a gap only as wide as
// itself, where on the line it would also have to clear both arrowheads.
func linkLabel(c decker.Ctx, p *decker.Pixels, from, to decker.Rect, s string, prog float64) {
	_, fy := from.Center()
	_, ty := to.Center()
	size := float64(c.SmallText(c.Theme.Body))
	decker.LineLabel(c, p, c.Theme.Body, s, (from.Right()+to.X)/2, (fy+ty)/2-size*0.75,
		c.Theme.Muted, decker.Clamp01(prog*2-1))
}

// labelGap is how wide a gap must be for linkLabel to fit s in it.
func labelGap(c decker.Ctx, s string) float64 {
	size := c.SmallText(c.Theme.Body)
	return c.Theme.Body.Measure(s, size) + float64(size)*1.2
}

// tierColor is the outline color of a tier's boxes.
func tierColor(th *decker.Theme, tier int) decker.RGB {
	return []decker.RGB{th.Accent, th.Accent2, th.Good}[tier%3]
}

// nodeIndex finds a node by key. A key missing from archNodes is a typo in
// the data, so it panics, which the deck's tests catch.
func nodeIndex(key string) int {
	for i, n := range archNodes {
		if n.key == key {
			return i
		}
	}
	panic("architecture: no node " + key)
}

// The zoom slide opens one service. Its neighbors keep their keys and glide
// to new rects; the nodes it doesn't place fade out.

// zoomKey is the service the second slide opens, and zoomParts what's
// inside it, top to bottom.
const zoomKey = "payments"

var zoomParts = []string{"api", "fraud", "ledger"}

// zoomLinks are the opened service's connections on the second slide: what
// calls it goes on the left, what it calls on the right.
var zoomLinks = []link{
	{"gateway", "payments", ""},
	{"payments", "kafka", "pub"},
	{"payments", "postgres", "SQL"},
}

func architectureZoomSlide() decker.Slide {
	return decker.Slide{Title: "Inside payments", Transition: decker.TransitionMorph.Over(1),
		Steps: 2, // the zoom, then the path through the parts
		Notes: "Same boxes, new places: payments opens up. Next click walks a charge through it.",
		View: func(c decker.Ctx, sc *decker.Scene) {
			p, th := sc.Px, c.Theme
			area := heading(c, p, "Inside payments")
			z := layoutZoom(c, area, true)
			if !c.Fits("zoomed architecture", area, z.needW, z.needH) {
				// As on the slide before: drop the labels, keep the names.
				z = layoutZoom(c, area, false)
			}

			// Connectors draw on once the morph has landed. Each runs
			// straight across from the band of the opened panel level with
			// the box at its other end, so the links don't share an anchor.
			for _, k := range zoomLinks {
				from, to := z.rect(k.from), z.rect(k.to)
				if k.from == zoomKey {
					from = level(from, to)
				} else if k.to == zoomKey {
					to = level(to, from)
				}
				prog := decker.Ease(c.T-0.9, 0.5)
				decker.Connector{From: from, To: to, Head: decker.HeadArrow, Color: th.Muted, Prog: prog}.Draw(c, p)
				if z.labels && k.label != "" {
					linkLabel(c, p, from, to, k.label, prog)
				}
			}

			for _, key := range z.keys {
				n := archNodes[nodeIndex(key)]
				r := z.rect(key)
				if key == zoomKey {
					sc.Place(key, r, func(p *decker.Pixels, r decker.Rect) { drawOpened(c, p, r, n) })
					continue
				}
				sc.Place(key, r, func(p *decker.Pixels, r decker.Rect) { drawNode(c, p, r, n, 1) })
			}
		}}
}

// zoomLayout places the opened service in the middle, what calls it on the
// left and what it writes to on the right.
type zoomLayout struct {
	keys         []string
	rects        []decker.Rect
	labels       bool // whether the gaps leave room for connector labels
	needW, needH float64
}

func (z zoomLayout) rect(key string) decker.Rect {
	for i, k := range z.keys {
		if k == key {
			return z.rects[i]
		}
	}
	return decker.Rect{}
}

func layoutZoom(c decker.Ctx, r decker.Rect, labels bool) zoomLayout {
	th := c.Theme
	txt := decker.Text{Font: th.Body, Size: c.SmallText(th.Body)}
	_, lineH := txt.Measure("Ag")
	pad := float64(txt.Size) * 0.6
	boxH := lineH * boxLines

	// Sort the neighbors into the two sides. Each side is as wide as its
	// widest name, and each gap as wide as the labels crossing it need.
	var left, right []string
	leftW, rightW := 0.0, 0.0
	leftGap, rightGap := c.Unit(0.1), c.Unit(0.1)
	for _, k := range zoomLinks {
		gap := c.Unit(0.1)
		if labels {
			gap = labelGap(c, k.label)
		}
		if k.to == zoomKey {
			w, _ := txt.Measure(k.from)
			left, leftW, leftGap = append(left, k.from), max(leftW, w+2*pad), max(leftGap, gap)
		} else {
			w, _ := txt.Measure(k.to)
			right, rightW, rightGap = append(right, k.to), max(rightW, w+2*pad), max(rightGap, gap)
		}
	}
	in := openedNeeds(c, txt)
	most := float64(max(len(left), len(right)))
	z := zoomLayout{labels: labels}
	z.needW = leftW + leftGap + in.W + rightGap + rightW
	z.needH = max(in.H, most*boxH+(most-1)*lineH)

	// The middle column takes what the sides and gaps leave. The gaps are
	// columns too, so each can have its own width.
	mid := max(r.W-leftW-leftGap-rightGap-rightW, 0)
	cols := r.Cols(0, leftW, leftGap, mid, rightGap, rightW)
	z.add(zoomKey, cols[2])
	for _, side := range []struct {
		keys []string
		col  decker.Rect
	}{{left, cols[0]}, {right, cols[4]}} {
		ones := make([]float64, len(side.keys))
		for i := range ones {
			ones[i] = 1
		}
		for i, slot := range side.col.Rows(lineH, ones...) {
			z.add(side.keys[i], slot.Anchor(slot.W, boxH, 0.5, 0.5))
		}
	}
	return z
}

func (z *zoomLayout) add(key string, r decker.Rect) {
	z.keys = append(z.keys, key)
	z.rects = append(z.rects, r)
}

// level returns the band of big level with r: a connector from it to r
// runs straight across.
func level(big, r decker.Rect) decker.Rect { return decker.NewRect(big.X, r.Y, big.W, r.H) }

// openedNeeds is the smallest rect drawOpened can lay its parts out in.
func openedNeeds(c decker.Ctx, txt decker.Text) decker.Rect {
	_, lineH := txt.Measure("Ag")
	pad := float64(txt.Size) * 0.7
	w := 0.0
	for _, s := range append([]string{zoomKey}, zoomParts...) {
		sw, _ := txt.Measure(s)
		w = max(w, sw+2*pad)
	}
	n := float64(len(zoomParts))
	return decker.Rect{W: w + 2*pad, H: lineH*1.4 + n*lineH*1.6 + (n-1)*lineH*0.7 + pad}
}

// drawOpened draws the opened service filling r: a panel with its name on
// top and its parts stacked inside, joined on the second build. Parts that
// don't fit r (as on the way in, while the morph grows the panel) are left
// out rather than squeezed.
func drawOpened(c decker.Ctx, p *decker.Pixels, r decker.Rect, n node) {
	th := c.Theme
	edge := tierColor(th, n.tier)
	stroke := max(c.Unit(0.007), 1)
	rad := min(c.Unit(0.03), r.H/4)
	p.RoundRect(r.X, r.Y, r.W, r.H, rad, 0, th.Panel, 1)
	p.RoundRect(r.X, r.Y, r.W, r.H, rad, stroke, edge, 1)

	txt := decker.Text{Font: th.Body, Size: c.SmallText(th.Body), Color: th.Text}
	_, lineH := txt.Measure("Ag")
	pad := float64(txt.Size) * 0.7
	inner := r.Inset(pad, pad/2)
	title, rest := inner.CutTop(lineH * 1.4)
	name := txt
	name.Color = edge
	name.DrawMid(p, n.key, title.X, title.Y+title.H/2)

	// A plain comparison rather than c.Fits: while the morph grows the
	// panel, falling short is expected, not a problem to report. The
	// slide's own c.Fits covers the panel's settled rect.
	need := openedNeeds(c, txt)
	if r.W < need.W || r.H < need.H {
		return
	}
	weights := make([]float64, len(zoomParts))
	for i := range weights {
		weights[i] = 1
	}
	slots := rest.Inset(0, pad/2).Rows(lineH*0.7, weights...)
	parts := make([]decker.Rect, len(slots))
	for i, s := range slots {
		parts[i] = s.Anchor(min(s.W, need.W*1.4), min(s.H, lineH*1.6), 0.5, 0.5)
	}
	for i := 1; i < len(parts); i++ {
		decker.Connector{From: parts[i-1], To: parts[i], Head: decker.HeadArrow, Color: th.Warn,
			Prog: decker.Ease(c.Since(1)-0.3*float64(i-1), 0.4)}.Draw(c, p)
	}
	part := txt
	part.Align = decker.Center
	for i, s := range parts {
		lit := decker.Ease(c.Since(1)-0.3*float64(i), 0.3)
		rr := min(c.Unit(0.02), s.H/3)
		p.RoundRect(s.X, s.Y, s.W, s.H, rr, 0, decker.Mix(th.Background, th.Warn, 0.12*lit), 1)
		p.RoundRect(s.X, s.Y, s.W, s.H, rr, stroke, decker.Mix(th.Faint, th.Warn, lit), 1)
		part.DrawMid(p, zoomParts[i], s.X+s.W/2, s.Y+s.H/2)
	}
}

package main

// Recipe: a request flow, as a sequence diagram on a time axis.
//
// What it shows: a request passing between participants, one message per
// build: each call an arrow drawing on, each reply a dashed arrow back.
// Messages sit at the time they were sent, down a millisecond axis, so the
// gaps between them mean something; a bracket measures the slow part.
//
// How it's built:
//
//  1. Layout. layoutFlow is a pure function of the rect and the messages.
//     The lifelines are spread across what the headers, the time axis and
//     the bracket leave (all measured with Text.Measure); the messages are
//     placed down a NiceScale of their send times.
//  2. Check. Two messages between the same pair of participants need a
//     label's height between them, which sets how tall the time axis must
//     be. c.Fits compares that with the rect. When it doesn't fit, the
//     layout drops the time axis and the bracket and spaces the messages
//     evenly: the order survives, the timing goes.
//  3. Draw. Headers and dashed lifelines, then the activation bars (a
//     participant is busy from a call until its reply), then the arrows and
//     their labels, then the bracket.
//  4. Reveal. Message i draws on at build i; activation bars stretch down
//     with the flow, and the bracket appears with the reply that closes it.
//
// Knobs: flowActors, checkoutFlow (who, what, when), flowBracket (which
// two messages to measure between), and the arrow timing in drawFlow.

import (
	"strconv"

	"github.com/austinlparker/decker"
)

// flowActors are the participants, left to right.
var flowActors = []string{"client", "api", "orders", "db"}

// message is one arrow in the flow.
type message struct {
	from, to int     // indexes into flowActors
	label    string  // keep it short: it sits between two lifelines
	at       float64 // ms since the request began
	reply    bool    // drawn dashed
}

var checkoutFlow = []message{
	{0, 1, "POST", 0, false},
	{1, 2, "create", 5, false},
	{2, 3, "INSERT", 11, false},
	{3, 2, "ok", 29, true},
	{2, 1, "201", 34, true},
	{1, 0, "201", 38, true},
}

// flowBracket names the two messages the bracket measures between.
var flowBracket = [2]int{2, 3}

func requestFlowSlide() decker.Slide {
	return decker.Slide{Title: "Request flow", Section: "Request flow", Transition: decker.TransitionPush,
		Steps: len(checkoutFlow), // one build per message
		Notes: "Follow the request down. The bracket shows the insert is most of the request's time.",
		View: func(c decker.Ctx, sc *decker.Scene) {
			area := heading(c, sc.Px, "Request flow")
			lay := layoutFlow(c, area, checkoutFlow, true)
			if !c.Fits("request flow", area, lay.needW, lay.needH) {
				// The times need a taller axis than the room allows: keep the
				// order and drop the timing.
				lay = layoutFlow(c, area, checkoutFlow, false)
			}
			drawFlow(c, sc.Px, lay)
		}}
}

// flowLayout is where everything goes, worked out before drawing.
type flowLayout struct {
	msgs  []message
	text  decker.Text // one size for every label
	lineH float64     // the height of a line of text
	gap   float64     // the spacing unit
	actW  float64     // the width of an activation bar

	heads    []decker.Rect // the participants' header boxes
	xs       []float64     // each lifeline's x
	lifeEnd  float64       // where the lifelines stop
	ys       []float64     // each message's y
	timed    bool          // whether ys follow the send times
	ms       decker.Scale  // ms onto y, when timed
	axisX    float64       // the time axis, when timed
	bracketX float64       // the bracket's line, when timed

	needW, needH float64 // the room the layout needs at the least
}

func layoutFlow(c decker.Ctx, r decker.Rect, msgs []message, timed bool) flowLayout {
	th := c.Theme
	l := flowLayout{msgs: msgs, timed: timed, gap: c.Unit(0.02), actW: c.Unit(0.025)}
	l.text = decker.Text{Font: th.Body, Size: c.SmallText(th.Body)}
	_, l.lineH = l.text.Measure("Ag")
	pad := float64(l.text.Size) * 0.7
	measure := func(s string) float64 { w, _ := l.text.Measure(s); return w }

	headW, labelW := 0.0, 0.0
	for _, a := range flowActors {
		headW = max(headW, measure(a)+2*pad)
	}
	for _, m := range msgs {
		labelW = max(labelW, measure(m.label))
	}
	end := 0.0
	for _, m := range msgs {
		end = max(end, m.at)
	}

	// The room each side needs: half a header, and when timed, the tick
	// labels on the left and the bracket on the right.
	left, right := headW/2, headW/2
	probe := decker.NiceScale(0, end, 5, 0, 1)
	tickW := measure(probe.Label(probe.Max) + "ms")
	if timed {
		left = max(left, tickW+2*l.gap)
		right = max(right, l.actW/2+3*l.gap+measure(bracketLabel(msgs)))
	}
	spacing := max(headW+l.gap, labelW+2*l.gap+l.actW)
	n := float64(len(flowActors))
	l.needW = left + right + (n-1)*spacing

	// Lifelines spread evenly over what the sides leave.
	l.xs = make([]float64, len(flowActors))
	l.heads = make([]decker.Rect, len(flowActors))
	headH := l.lineH * 1.8
	for i, a := range flowActors {
		l.xs[i] = r.X + left + float64(i)*(r.W-left-right)/(n-1)
		w := measure(a) + 2*pad
		l.heads[i] = decker.NewRect(l.xs[i]-w/2, r.Y, w, headH)
	}

	// Messages run from below the headers, leaving a label's room above the
	// first, to the bottom, leaving half a tick label below the last.
	top := r.Y + headH + l.lineH + 2*l.gap
	bottom := r.Bottom() - l.lineH/2
	margins := (top - r.Y) + (r.Bottom() - bottom)
	l.lifeEnd = r.Bottom()
	minDY := l.lineH + l.gap // a label above an arrow
	l.ys = make([]float64, len(msgs))
	if timed {
		// Two messages between the same pair of lifelines need minDY between
		// them; that sets the pixels each millisecond needs.
		perMs := 0.0
		for i, a := range msgs {
			for _, b := range msgs[i+1:] {
				if pairOf(a) == pairOf(b) && b.at > a.at {
					perMs = max(perMs, minDY/(b.at-a.at))
				}
			}
		}
		l.ms = decker.NiceScale(0, end, max(int((bottom-top)/(l.lineH*1.5)), 2), top, bottom)
		l.needH = margins + (l.ms.Max-l.ms.Min)*perMs
		for i, m := range msgs {
			l.ys[i] = l.ms.At(m.at)
		}
		l.axisX = r.X + tickW + l.gap
		l.bracketX = l.xs[len(l.xs)-1] + l.actW/2 + l.gap
	} else {
		l.needH = margins + float64(len(msgs)-1)*minDY
		for i := range msgs {
			l.ys[i] = top + float64(i)*(bottom-top)/float64(max(len(msgs)-1, 1))
		}
	}
	return l
}

func drawFlow(c decker.Ctx, p *decker.Pixels, l flowLayout) {
	th := c.Theme
	stroke := max(c.Unit(0.006), 1)

	// The time axis: a line with ticks and labels, on the left.
	if l.timed {
		a := decker.Ease(c.T-0.2, 0.5)
		p.Rect(l.axisX, l.ms.From, stroke, l.ms.To-l.ms.From, decker.Mix(th.Background, th.Faint, a), 1)
		tick := l.text
		tick.Color, tick.Align = decker.Mix(th.Background, th.Muted, a), decker.Right
		for v := range l.ms.Ticks() {
			y := l.ms.At(v)
			p.Rect(l.axisX-l.gap/2, y, l.gap/2, stroke, decker.Mix(th.Background, th.Faint, a), 1)
			tick.DrawMid(p, l.ms.Label(v)+"ms", l.axisX-l.gap, y)
		}
	}

	// Headers and dashed lifelines.
	for i, h := range l.heads {
		a := decker.Ease(c.T-0.1*float64(i), 0.4)
		x := l.xs[i]
		p.DashedLine(x, h.Bottom(), x, h.Bottom()+(l.lifeEnd-h.Bottom())*a, stroke, c.Unit(0.02), c.Unit(0.015), th.Faint, 1)
		rad := min(c.Unit(0.02), h.H/3)
		p.RoundRect(h.X, h.Y, h.W, h.H, rad, 0, th.Panel, a)
		p.RoundRect(h.X, h.Y, h.W, h.H, rad, stroke, decker.Mix(th.Background, th.Accent, a), 1)
		head := l.text
		head.Color, head.Align = decker.Mix(th.Background, th.Text, a), decker.Center
		head.DrawMid(p, flowActors[i], x, h.Y+h.H/2)
	}

	// Activation bars: a participant is busy from the call it receives
	// until it replies, and stretches down with the flow until then.
	for _, act := range activations(l.msgs) {
		if !c.Reached(act.from) {
			continue
		}
		k := min(c.Step, act.to)
		y1 := l.ys[k]
		if k > act.from {
			y1 = decker.Lerp(l.ys[k-1], l.ys[k], decker.Ease(c.Since(k), 0.5))
		}
		x := l.xs[act.who] - l.actW/2
		y0 := l.ys[act.from]
		p.Rect(x, y0, l.actW, max(y1-y0, stroke), decker.Mix(th.Panel, th.Accent, 0.35), 1)
		p.RoundRect(x, y0, l.actW, max(y1-y0, stroke), 0, stroke, th.Accent, 1)
	}

	// Messages: an arrow drawing on, its label above it, a reply dashed.
	for i, m := range l.msgs {
		if !c.Reached(i) {
			continue
		}
		t := c.Since(i)
		y := l.ys[i]
		col := th.Text
		if m.reply {
			col = th.Muted
		}
		at := func(who int) decker.Rect { return decker.NewRect(l.xs[who]-l.actW/2, y, l.actW, 0) }
		decker.Connector{From: at(m.from), To: at(m.to), Head: decker.HeadArrow, Dashed: m.reply,
			Color: col, Prog: decker.Ease(t, 0.5)}.Draw(c, p)
		lab := l.text
		lab.Color, lab.Align = col, decker.Center
		lab.FX = decker.FadeUp(t-0.2, 0.3, lab.Size)
		lab.DrawMid(p, m.label, (l.xs[m.from]+l.xs[m.to])/2, y-l.gap/2-l.lineH/2)
	}

	// The bracket: a line between two messages' times, with its length
	// written beside it, once the second message has been sent.
	if b := flowBracket; l.timed && c.Reached(b[1]) {
		t := c.Since(b[1])
		y0, y1 := l.ys[b[0]], l.ys[b[1]]
		grow := decker.Ease(t, 0.5)
		x, tk := l.bracketX, l.gap
		w := max(c.Unit(0.008), 1.5)
		p.Rect(x+tk, y0, w, (y1-y0)*grow, th.Accent2, 1)
		p.Rect(x, y0, tk+w, w, th.Accent2, 1)
		if grow >= 1 {
			p.Rect(x, y1-w, tk+w, w, th.Accent2, 1)
		}
		lab := l.text
		lab.Color = th.Accent2
		lab.FX = decker.FadeUp(t-0.4, 0.3, lab.Size)
		lab.DrawMid(p, bracketLabel(l.msgs), x+tk+w+l.gap, (y0+y1)/2)
	}
}

// bracketLabel is the time between the bracket's two messages, like "18ms".
func bracketLabel(msgs []message) string {
	b := flowBracket
	return strconv.FormatFloat(msgs[b[1]].at-msgs[b[0]].at, 'f', -1, 64) + "ms"
}

// activation is a span of messages during which a participant is busy.
type activation struct{ who, from, to int }

// activations pairs each call with the reply that answers it; the
// participant who starts the flow is busy until the last message.
func activations(msgs []message) []activation {
	if len(msgs) == 0 {
		return nil
	}
	out := []activation{{msgs[0].from, 0, len(msgs) - 1}}
	for i, m := range msgs {
		if m.reply {
			continue
		}
		for j := i + 1; j < len(msgs); j++ {
			if r := msgs[j]; r.reply && r.from == m.to && r.to == m.from {
				out = append(out, activation{m.to, i, j})
				break
			}
		}
	}
	return out
}

// pairOf names the two lifelines a message runs between, in either
// direction.
func pairOf(m message) [2]int { return [2]int{min(m.from, m.to), max(m.from, m.to)} }

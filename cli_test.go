package decker

import (
	"strings"
	"testing"
)

// runCLI parses args for d and runs the command, as Main does, without
// exiting.
func runCLI(d *Deck, args ...string) error {
	_, ctx, err := parseArgs(d, args)
	if err != nil {
		return err
	}
	return ctx.Run(d)
}

func TestCommandLine(t *testing.T) {
	for _, tc := range []struct {
		args    []string
		command string
		check   func(any) bool
	}{
		{nil, "live", func(c any) bool { l := c.(liveCmd); return l.Slide == 1 && l.FPS == 60 && !l.Dev }},
		{[]string{"--dev", "--slide", "3", "--step=2"}, "live", func(c any) bool {
			l := c.(liveCmd)
			return l.Dev && l.Slide == 3 && l.Step == 2 && l.Socket == defaultSocket(testDeck().Name)
		}},
		{[]string{"review", "out"}, "review <dir>", func(c any) bool { return c.(reviewCmd).Dir == "out" }},
		{[]string{"snapshot", "-w", "40", "-h", "12", "-t", "0.5", "--png", "f.png"}, "snapshot", func(c any) bool {
			s := c.(snapshotCmd)
			return s.Width == 40 && s.Height == 12 && s.Time == 0.5 && s.PNG == "f.png" && s.Slide == 1
		}},
		{[]string{"sheet", "s.png"}, "sheet <file>", func(c any) bool {
			s := c.(sheetCmd)
			return s.File == "s.png" && s.Shrink == 4 && s.Time == Settled && s.Width == 120 && s.Height == 36
		}},
		{[]string{"video", "v.mp4", "--until", "3"}, "video <file>", func(c any) bool {
			v := c.(videoCmd)
			return v.File == "v.mp4" && v.Until == 3 && v.Size == "1920x1080" && v.FPS == 60
		}},
		{[]string{"present", "--previews", "cells"}, "present", func(c any) bool { return c.(presentCmd).Previews == "cells" }},
	} {
		_, ctx, err := parseArgs(testDeck(), tc.args)
		if err != nil {
			t.Errorf("%q: %v", tc.args, err)
			continue
		}
		if ctx.Command() != tc.command {
			t.Errorf("%q ran %q, want %q", tc.args, ctx.Command(), tc.command)
			continue
		}
		if c := ctx.Selected().Target.Interface(); !tc.check(c) {
			t.Errorf("%q parsed as %+v", tc.args, c)
		}
	}
}

// The parser rejects what the old flags let through silently: two modes at
// once, a flag that belongs to another mode, a value out of range.
func TestCommandLineRejects(t *testing.T) {
	for _, args := range [][]string{
		{"review"},                           // no directory
		{"video", "a.mp4", "sheet", "b.png"}, // two commands
		{"review", "out", "--shrink", "2"},   // another command's flag
		{"sheet", "s.png", "--shrink", "0"},  // would divide by zero
		{"sheet", "s.png", "-w", "0"},        // no frame
		{"present", "--previews", "typo"},    // not a mode
		{"present", "--presentation-font-size", "0"},
		{"--fps", "0"},
		{"video", "v.mp4", "--fps", "0"},
		{"--png", "f.png"}, // a snapshot flag, without snapshot
	} {
		if _, _, err := parseArgs(testDeck(), args); err == nil {
			t.Errorf("%q: accepted", args)
		}
	}
	for _, args := range [][]string{{"-review", "out"}, {"-snapshot"}, {"snapshot", "-slide=3"}} {
		if _, _, err := parseArgs(testDeck(), args); err == nil || !strings.Contains(err.Error(), "two dashes") {
			t.Errorf("%q: %v, want the new form explained", args, err)
		}
	}
	if err := runCLI(testDeck(), "video", "v.mp4", "--size", "big"); err == nil || !strings.Contains(err.Error(), "--size") {
		t.Errorf("--size big: %v", err)
	}
}

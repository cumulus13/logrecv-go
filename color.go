// File: color.go
// Author: Hadi Cahyadi <cumulus13@gmail.com>
// Date: 2026-10-28
// Description: 
// License: MIT

package main

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
)

const reset = "\x1b[0m"

type colorMode int

const (
	modeNone colorMode = iota
	mode256
	modeTrue
)

func (m colorMode) String() string {
	switch m {
	case modeTrue:
		return "truecolor"
	case mode256:
		return "256"
	}
	return "none"
}

type rgb struct{ r, g, b uint8 }

// parseHex accepts #RRGGBB, RRGGBB, #RGB (extra leading '#' tolerated).
func parseHex(s string) (rgb, error) {
	h := strings.TrimLeft(strings.TrimSpace(s), "#")
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	if len(h) != 6 {
		return rgb{}, fmt.Errorf("invalid hex color %q (want #RRGGBB or #RGB)", s)
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return rgb{}, fmt.Errorf("invalid hex color %q", s)
	}
	return rgb{uint8(v >> 16), uint8(v >> 8), uint8(v)}, nil
}

func to256(c rgb) int {
	r, g, b := int(c.r), int(c.g), int(c.b)
	if r == g && g == b {
		switch {
		case r < 8:
			return 16
		case r > 248:
			return 231
		}
		return 232 + (r-8)*24/247
	}
	q := func(v int) int { return (v*5 + 127) / 255 }
	return 16 + 36*q(r) + 6*q(g) + q(b)
}

func sgr(c rgb, bg bool, m colorMode) string {
	base := 38
	if bg {
		base = 48
	}
	if m == modeTrue {
		return fmt.Sprintf("\x1b[%d;2;%d;%d;%dm", base, c.r, c.g, c.b)
	}
	return fmt.Sprintf("\x1b[%d;5;%dm", base, to256(c))
}

// Style is a precompiled ANSI prefix (empty = no styling).
type Style struct{ pre string }

func (s Style) Paint(text string) string {
	if s.pre == "" || text == "" {
		return text
	}
	return s.pre + text + reset
}

// newStyle always validates the hex values, even when colors are disabled,
// so config mistakes surface regardless of terminal capabilities.
func newStyle(m colorMode, fg, bg string) (Style, error) {
	var sb strings.Builder
	for _, p := range []struct {
		name, v string
		bg      bool
	}{{"fg", fg, false}, {"bg", bg, true}} {
		if strings.TrimSpace(p.v) == "" {
			continue
		}
		c, err := parseHex(p.v)
		if err != nil {
			return Style{}, fmt.Errorf("%s: %w", p.name, err)
		}
		if m != modeNone {
			sb.WriteString(sgr(c, p.bg, m))
		}
	}
	return Style{pre: sb.String()}, nil
}

func stdoutIsTTY() bool {
	fi, err := os.Stdout.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func detectMode(flagVal string) (colorMode, error) {
	switch strings.ToLower(strings.TrimSpace(flagVal)) {
	case "never", "none", "off", "false":
		return modeNone, nil
	case "always", "truecolor", "24bit", "true":
		enableVT()
		return modeTrue, nil
	case "256":
		enableVT()
		return mode256, nil
	case "auto", "":
		if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" || !stdoutIsTTY() {
			return modeNone, nil
		}
		if !enableVT() {
			return modeNone, nil
		}
		ct := strings.ToLower(os.Getenv("COLORTERM"))
		if ct == "truecolor" || ct == "24bit" || runtime.GOOS == "windows" {
			return modeTrue, nil
		}
		if os.Getenv("TERM") == "" {
			return modeTrue, nil
		}
		return mode256, nil
	}
	return modeNone, fmt.Errorf("invalid -color %q (auto|always|never|256)", flagVal)
}

package styles

import (
	"image/color"
	"math"
)

// The palette's polarity, measured rather than declared.
//
// A Theme carries no `Dark bool` field on purpose: a declared polarity is a
// second copy of a fact the colors already state, and the two drift the moment
// someone retunes a background without touching the flag. `Background` *is* the
// claim — kubecom paints it across the whole screen (D249) — so reading its
// luminance is reading the palette's own answer.
//
// The arithmetic is WCAG 2.1's, and it lives here rather than in a test file
// because two callers now need the same answer: the registry guard that admits a
// palette (`TestBuiltinThemesAreDarkAndLegible`) and the shell, which compares
// the palette's polarity against the terminal's actual background (D250). Two
// implementations of the same threshold would let a palette pass the guard and
// then be reported to the user as the other polarity.

// darkLuminance is the boundary between a dark background and a light one. It is
// the same 0.2 the registry's admission guard uses for `Selection` and
// `StatusBarBg`, so "dark enough to be admitted" and "dark" are one threshold and
// cannot disagree.
const darkLuminance = 0.2

// RelativeLuminance is WCAG 2.1's L for c, over the sRGB channels color.Color
// hands back (16-bit, alpha-premultiplied — every theme color is opaque, so the
// premultiply is a no-op). 0 is black, 1 is white.
func RelativeLuminance(c color.Color) float64 {
	if c == nil {
		return 0
	}
	r, g, b, _ := c.RGBA()
	lin := func(v uint32) float64 {
		s := float64(v) / 65535
		if s <= 0.03928 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}

// ContrastRatio is WCAG 2.1's ratio between two colors: 1 (identical) to 21
// (black on white). Order-independent.
func ContrastRatio(a, b color.Color) float64 {
	la, lb := RelativeLuminance(a), RelativeLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// IsDark reports whether c is a dark background. A nil color is reported dark,
// which is the conservative answer: nil means "unknown / the terminal's own", and
// kubecom's whole registry is dark, so treating it as dark is the reading that
// raises no alarm about a screen nobody has measured.
func IsDark(c color.Color) bool {
	return RelativeLuminance(c) <= darkLuminance
}

// SameColor reports whether two colors are the same 8-bit sRGB triple. It exists
// because the two sides being compared come from different places: a Theme's
// value is a lipgloss.Color parsed from a hex literal, while the terminal's own
// answer to an OSC 11 query is whatever precision that terminal chose to report
// (`rgb:28/28/28` and `rgb:2828/2828/2828` are the same color said two ways).
// Comparing at 8 bits is what both round-trip to; comparing the interface values
// or the raw 16-bit channels would call those two answers different.
func SameColor(a, b color.Color) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	return ar>>8 == br>>8 && ag>>8 == bg>>8 && ab>>8 == bb>>8
}

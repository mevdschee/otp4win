package main

import "math"

// rgb is a 0xRRGGBB color.
type rgb uint32

func (c rgb) r() uint8 { return uint8(c >> 16) }
func (c rgb) g() uint8 { return uint8(c >> 8) }
func (c rgb) b() uint8 { return uint8(c) }

// luminance is the WCAG relative luminance.
func (c rgb) luminance() float64 {
	ch := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.03928 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*ch(c.r()) + 0.7152*ch(c.g()) + 0.0722*ch(c.b())
}

// contrast is the WCAG contrast ratio between a and b (1 to 21).
func contrast(a, b rgb) float64 {
	la, lb := a.luminance(), b.luminance()
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// mix blends a toward b by t (0 = a, 1 = b).
func mix(a, b rgb, t float64) rgb {
	m := func(x, y uint8) rgb { return rgb(math.Round(float64(x) + (float64(y)-float64(x))*t)) }
	return m(a.r(), b.r())<<16 | m(a.g(), b.g())<<8 | m(a.b(), b.b())
}

// readableOn returns black or white, whichever contrasts most with bg.
func readableOn(bg rgb) rgb {
	if contrast(0x000000, bg) >= contrast(0xFFFFFF, bg) {
		return 0x000000
	}
	return 0xFFFFFF
}

// largeTextOn returns white when it is readable as large text on bg (WCAG
// 3:1 for bold text of 14pt and up), like Windows puts white text on its
// accent color; otherwise black. Used for the letter in the issuer circle.
func largeTextOn(bg rgb) rgb {
	if contrast(0xFFFFFF, bg) >= 3 {
		return 0xFFFFFF
	}
	return 0x000000
}

// ensureContrast moves fg toward black or white until it reaches the
// minimum contrast ratio against bg, so odd system colors stay readable.
func ensureContrast(fg, bg rgb, minimum float64) rgb {
	target := readableOn(bg)
	for t := 0.0; t <= 1 && contrast(fg, bg) < minimum; t += 0.05 {
		fg = mix(fg, target, t)
	}
	return fg
}

// fix applies the minimum contrast ratios to a card's colors: 4.5:1 for
// text, 3:1 for the circle and the countdown bar.
func (c cardColors) fix() cardColors {
	c.text = ensureContrast(c.text, c.fill, 7)
	c.subtext = ensureContrast(c.subtext, c.fill, 4.5)
	c.iconFill = ensureContrast(c.iconFill, c.fill, 3)
	c.iconText = ensureContrast(c.iconText, c.iconFill, 3) // large bold text
	c.bar = ensureContrast(c.bar, c.track, 3)
	c.warn = ensureContrast(c.warn, c.track, 3)
	return c
}

// sysColors are the Windows system colors the light and high contrast
// palettes are built from (GetSysColor).
type sysColors struct {
	window, windowText, grayText, btnFace, highlight, highlightText rgb
}

// defaultSysColors are the Windows 10/11 defaults.
var defaultSysColors = sysColors{
	window:        0xFFFFFF,
	windowText:    0x000000,
	grayText:      0x6D6D6D,
	btnFace:       0xF0F0F0,
	highlight:     0x0078D7,
	highlightText: 0xFFFFFF,
}

// cardColors are the colors of a card in one state (normal or selected).
type cardColors struct {
	fill, border  rgb
	text, subtext rgb
	iconFill      rgb // issuer circle
	iconText      rgb
	bar, track    rgb // countdown bar
	warn          rgb // countdown bar in the last seconds
}

// palette holds all colors of the custom painted parts of the UI.
type palette struct {
	dark, highContrast bool
	background         rgb // window and list background
	text               rgb // text on the background (headers, status)
	edit, editText     rgb // search box in dark mode
	card, selected     cardColors
}

// buildPalette derives the palette from the Windows colors: the system
// colors in light mode, the Windows 11 dark colors in dark mode, and only
// system colors in high contrast mode.
func buildPalette(dark, highContrast bool, sys sysColors) palette {
	if highContrast {
		normal := cardColors{
			fill: sys.window, border: sys.windowText,
			text: sys.windowText, subtext: sys.windowText,
			iconFill: sys.windowText, iconText: sys.window,
			bar: sys.windowText, track: sys.window, warn: sys.windowText,
		}
		sel := cardColors{
			fill: sys.highlight, border: sys.highlightText,
			text: sys.highlightText, subtext: sys.highlightText,
			iconFill: sys.highlightText, iconText: sys.highlight,
			bar: sys.highlightText, track: sys.highlight, warn: sys.highlightText,
		}
		return palette{highContrast: true, dark: sys.window.luminance() < 0.5,
			background: sys.window, text: sys.windowText,
			edit: sys.window, editText: sys.windowText, card: normal, selected: sel}
	}
	if dark {
		// Windows 11 dark mode: Mica-like page, layered cards and the light
		// accent variant that Windows uses on dark backgrounds.
		const accent rgb = 0x60CDFF
		normal := cardColors{
			fill: 0x2B2B2B, border: 0x3A3A3A,
			text: 0xFFFFFF, subtext: 0xC8C8C8,
			iconFill: accent, iconText: largeTextOn(accent),
			bar: accent, track: 0x4A4A4A, warn: 0xFF99A4,
		}
		sel := normal
		sel.fill = mix(normal.fill, accent, 0.18)
		sel.border = accent
		sel.track = mix(sel.fill, 0xFFFFFF, 0.15)
		return palette{dark: true, background: 0x202020, text: 0xFFFFFF,
			edit: 0x2D2D2D, editText: 0xFFFFFF, card: normal.fix(), selected: sel.fix()}
	}
	accent := sys.highlight
	normal := cardColors{
		fill: sys.window, border: mix(sys.window, sys.windowText, 0.12),
		text: sys.windowText, subtext: sys.grayText,
		iconFill: accent, iconText: largeTextOn(accent),
		bar: accent, track: mix(sys.window, sys.windowText, 0.12), warn: 0xC42B1C,
	}
	sel := normal
	sel.fill = mix(sys.window, accent, 0.15) // like Explorer's selection
	sel.border = accent
	sel.subtext = mix(sys.grayText, sys.windowText, 0.3)
	sel.track = sys.window
	return palette{background: sys.btnFace, text: ensureContrast(sys.windowText, sys.btnFace, 7),
		edit: sys.window, editText: sys.windowText, card: normal.fix(), selected: sel.fix()}
}

package main

import "testing"

func TestContrast(t *testing.T) {
	if got := contrast(0x000000, 0xFFFFFF); got < 20.9 || got > 21.1 {
		t.Errorf("black on white: %v", got)
	}
	if got := contrast(0x777777, 0x777777); got != 1 {
		t.Errorf("same color: %v", got)
	}
}

// Every palette must keep text readable (WCAG AA, 4.5:1) and the issuer
// circle and countdown bar visible (3:1), on normal and selected cards.
func TestPaletteContrast(t *testing.T) {
	highContrastBlack := sysColors{
		window: 0x000000, windowText: 0xFFFFFF, grayText: 0x3FF23F,
		btnFace: 0x000000, highlight: 0x1AEBFF, highlightText: 0x000000,
	}
	// Wine's system colors have a light gray text that is too faint.
	wine := defaultSysColors
	wine.grayText = 0xA0A0A0
	wine.highlight = 0x3399FF
	for name, p := range map[string]palette{
		"light":         buildPalette(false, false, defaultSysColors),
		"light (wine)":  buildPalette(false, false, wine),
		"dark":          buildPalette(true, false, defaultSysColors),
		"high contrast": buildPalette(false, true, highContrastBlack),
	} {
		for state, c := range map[string]cardColors{"normal": p.card, "selected": p.selected} {
			check := func(what string, fg, bg rgb, min float64) {
				if r := contrast(fg, bg); r < min {
					t.Errorf("%s %s: %s contrast %.2f < %.1f", name, state, what, r, min)
				}
			}
			check("text", c.text, c.fill, 7)
			check("subtext", c.subtext, c.fill, 4.5)
			check("icon letter", c.iconText, c.iconFill, 4.5)
			check("icon circle", c.iconFill, c.fill, 3)
			check("bar", c.bar, c.track, 3)
			check("bar warning", c.warn, c.track, 3)
		}
		if r := contrast(p.text, p.background); r < 7 {
			t.Errorf("%s: header text contrast %.2f", name, r)
		}
		if r := contrast(p.selected.fill, p.card.fill); r < 1.1 && !p.highContrast {
			t.Errorf("%s: selection not visible (%.2f)", name, r)
		}
	}
}

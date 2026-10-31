package main

import "testing"

func newCanvas(w, h int) *canvas {
	c := &canvas{pix: make([]uint32, w*h), w: w, h: h}
	c.fill(0xFFFFFF)
	return c
}

func (c *canvas) at(x, y int) rgb { return rgb(c.pix[y*c.w+x]) }

func TestFillCircle(t *testing.T) {
	c := newCanvas(20, 20)
	c.fillCircle(10, 10, 6, 0x000000)
	if got := c.at(10, 10); got != 0x000000 {
		t.Errorf("center: %06x", got)
	}
	if got := c.at(0, 0); got != 0xFFFFFF {
		t.Errorf("outside: %06x", got)
	}
	// A pixel on the diagonal edge is partly covered: gray, not black or white.
	edge := c.at(5, 5)
	if edge == 0x000000 || edge == 0xFFFFFF {
		t.Errorf("edge pixel not anti-aliased: %06x", edge)
	}
	// The shape is symmetric.
	for _, p := range [][4]int{{5, 5, 14, 14}, {4, 10, 15, 10}, {10, 4, 10, 15}} {
		if a, b := c.at(p[0], p[1]), c.at(p[2], p[3]); a != b {
			t.Errorf("not symmetric: (%d,%d)=%06x (%d,%d)=%06x", p[0], p[1], a, p[2], p[3], b)
		}
	}
}

func TestFillRoundRect(t *testing.T) {
	c := newCanvas(30, 20)
	c.fillRoundRect(2, 2, 28, 18, 6, 0x0000FF)
	if got := c.at(15, 2); got != 0x0000FF {
		t.Errorf("straight top edge should be fully covered: %06x", got)
	}
	if got := c.at(2, 2); got != 0xFFFFFF {
		t.Errorf("rounded corner should stay empty: %06x", got)
	}
	if got := c.at(15, 1); got != 0xFFFFFF {
		t.Errorf("outside: %06x", got)
	}
	// Half-pixel edges are blended half way.
	c = newCanvas(10, 10)
	c.fillRoundRect(2.5, 2, 8, 8, 0, 0x000000)
	if got := c.at(2, 5); got != 0x808080 && got != 0x7F7F7F {
		t.Errorf("half covered pixel: %06x", got)
	}
}

func TestCardBorder(t *testing.T) {
	c := newCanvas(40, 30)
	c.card(0, 0, 40, 30, 8, 1, 0xEEEEEE, 0x000000)
	if got := c.at(20, 0); got != 0x000000 {
		t.Errorf("border: %06x", got)
	}
	if got := c.at(20, 15); got != 0xEEEEEE {
		t.Errorf("fill: %06x", got)
	}
}

func TestClipping(t *testing.T) {
	c := newCanvas(10, 10)
	c.fillCircle(0, 0, 20, 0x000000) // larger than the canvas, must not panic
	c.fillRoundRect(-5, -5, 3, 3, 2, 0x000000)
	if got := c.at(9, 9); got != 0x000000 {
		t.Errorf("got %06x", got)
	}
}

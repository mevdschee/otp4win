package main

import "math"

// canvas draws anti-aliased shapes into a 32-bit pixel buffer laid out like
// a top-down Windows DIB section (0x00RRGGBB per pixel). GDI cannot smooth
// the edges of circles and rounded rectangles, so the list paints its shapes
// here and only uses GDI for the text.
type canvas struct {
	pix  []uint32
	w, h int
}

func (c *canvas) blend(x, y int, col rgb, alpha float64) {
	if alpha <= 0 || x < 0 || y < 0 || x >= c.w || y >= c.h {
		return
	}
	i := y*c.w + x
	if alpha >= 1 {
		c.pix[i] = uint32(col)
		return
	}
	c.pix[i] = uint32(mix(rgb(c.pix[i]&0xFFFFFF), col, alpha))
}

// fill paints the whole canvas.
func (c *canvas) fill(col rgb) {
	for i := range c.pix {
		c.pix[i] = uint32(col)
	}
}

// fillRoundRect fills the rectangle from (x0,y0) to (x1,y1) with corners of
// radius r. Coordinates are in pixels and may be fractional; edge pixels are
// blended by how much of them the shape covers.
func (c *canvas) fillRoundRect(x0, y0, x1, y1, r float64, col rgb) {
	if x1 <= x0 || y1 <= y0 {
		return
	}
	r = max(0, min(r, (x1-x0)/2, (y1-y0)/2))
	cx, cy := (x0+x1)/2, (y0+y1)/2
	hx, hy := (x1-x0)/2-r, (y1-y0)/2-r
	for y := max(0, int(math.Floor(y0))); y < min(c.h, int(math.Ceil(y1))); y++ {
		for x := max(0, int(math.Floor(x0))); x < min(c.w, int(math.Ceil(x1))); x++ {
			// Signed distance from the pixel center to the rounded rectangle.
			qx := math.Abs(float64(x)+0.5-cx) - hx
			qy := math.Abs(float64(y)+0.5-cy) - hy
			d := math.Hypot(max(qx, 0), max(qy, 0)) + min(max(qx, qy), 0) - r
			c.blend(x, y, col, 0.5-d)
		}
	}
}

// fillCircle fills a circle with center (cx,cy) and radius r.
func (c *canvas) fillCircle(cx, cy, r float64, col rgb) {
	c.fillRoundRect(cx-r, cy-r, cx+r, cy+r, r, col)
}

// card paints a rounded rectangle with a border of width bw.
func (c *canvas) card(x0, y0, x1, y1, r, bw float64, fill, border rgb) {
	c.fillRoundRect(x0, y0, x1, y1, r, border)
	c.fillRoundRect(x0+bw, y0+bw, x1-bw, y1-bw, max(0, r-bw), fill)
}

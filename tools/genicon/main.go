// Command genicon renders the application icon (a key with the letters OTP)
// into the PNG files that go-winres embeds in the executable.
//
// Run from the repository root:
//
//	go run ./tools/genicon
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"

	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const master = 1024 // size of the supersampled master image

var (
	background = color.RGBA{0x1f, 0x4e, 0x79, 0xff}
	keyColor   = color.RGBA{0xf2, 0xc1, 0x4e, 0xff}
	textColor  = color.RGBA{0xff, 0xff, 0xff, 0xff}
)

func main() {
	img := render()
	for _, out := range []struct {
		name string
		size int
	}{
		{"winres/icon.png", 256},
		{"winres/icon48.png", 48},
		{"winres/icon32.png", 32},
		{"winres/icon16.png", 16},
	} {
		dst := image.NewRGBA(image.Rect(0, 0, out.size, out.size))
		draw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Src, nil)
		f, err := os.Create(out.name)
		if err != nil {
			log.Fatal(err)
		}
		if err := png.Encode(f, dst); err != nil {
			log.Fatal(err)
		}
		if err := f.Close(); err != nil {
			log.Fatal(err)
		}
	}
}

// render draws the icon on a master x master canvas, in units of 1/256 of
// the icon size so the coordinates match a 256px icon.
func render() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, master, master))
	s := float64(master) / 256

	fill := func(c color.RGBA, inside func(x, y float64) bool) {
		for py := 0; py < master; py++ {
			for px := 0; px < master; px++ {
				if inside((float64(px)+0.5)/s, (float64(py)+0.5)/s) {
					img.SetRGBA(px, py, c)
				}
			}
		}
	}

	// Rounded square background.
	fill(background, func(x, y float64) bool { return inRoundRect(x, y, 4, 4, 252, 252, 40) })

	// Key: ring-shaped bow on the left, shaft to the right with two teeth.
	fill(keyColor, func(x, y float64) bool {
		d := math.Hypot(x-72, y-84)
		bow := d <= 46 && d >= 20
		shaft := inRoundRect(x, y, 110, 72, 230, 96, 6)
		tooth1 := inRoundRect(x, y, 182, 90, 200, 128, 3)
		tooth2 := inRoundRect(x, y, 210, 90, 228, 120, 3)
		return bow || shaft || tooth1 || tooth2
	})

	// The letters OTP, centred in the lower half.
	tt, err := opentype.Parse(gobold.TTF)
	if err != nil {
		log.Fatal(err)
	}
	face, err := opentype.NewFace(tt, &opentype.FaceOptions{Size: 100 * s, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		log.Fatal(err)
	}
	defer face.Close()
	d := font.Drawer{Dst: img, Src: image.NewUniform(textColor), Face: face}
	width := d.MeasureString("OTP")
	d.Dot = fixed.Point26_6{
		X: (fixed.I(master) - width) / 2,
		Y: fixed.I(int(228 * s)),
	}
	d.DrawString("OTP")
	return img
}

// inRoundRect reports whether (x, y) lies inside the rectangle (x0, y0)-(x1, y1)
// with corners rounded by radius r.
func inRoundRect(x, y, x0, y0, x1, y1, r float64) bool {
	if x < x0 || x > x1 || y < y0 || y > y1 {
		return false
	}
	cx := math.Max(x0+r, math.Min(x, x1-r))
	cy := math.Max(y0+r, math.Min(y, y1-r))
	return math.Hypot(x-cx, y-cy) <= r
}

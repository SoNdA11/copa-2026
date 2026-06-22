package handlers

import (
	"image"
	"image/color"
	stddraw "image/draw"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"

	"golang.org/x/image/draw"
)

func processAvatar(src io.Reader, dst io.Writer, size int) error {
	img, _, err := image.Decode(src)
	if err != nil {
		return err
	}

	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	side := w
	if h < w {
		side = h
	}
	cx := (w - side) / 2
	cy := (h - side) / 2

	square := image.NewRGBA(image.Rect(0, 0, side, side))
	stddraw.Draw(square, square.Bounds(), img, image.Pt(cx, cy), stddraw.Src)

	radius := float64(side) / 2
	center := float64(side)/2 + 0.5
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			dx := float64(x) - center
			dy := float64(y) - center
			if dx*dx+dy*dy > radius*radius {
				square.SetRGBA(x, y, color.RGBA{0, 0, 0, 0})
			}
		}
	}

	resized := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.CatmullRom.Scale(resized, resized.Bounds(), square, square.Bounds(), draw.Over, nil)

	return png.Encode(dst, resized)
}

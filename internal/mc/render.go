package mc

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
)

const (
	DefaultSize = 128
	MaxSize     = 512
	MinSize     = 1
)

func EncodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encode PNG: %w", err)
	}
	return buf.Bytes(), nil
}

func ExtractHelm(skin image.Image) image.Image {
	helm := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	draw.Draw(helm, helm.Bounds(), skin, image.Pt(8, 8), draw.Src)
	hat := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	draw.Draw(hat, hat.Bounds(), skin, image.Pt(40, 8), draw.Src)
	draw.Draw(helm, helm.Bounds(), hat, image.Pt(0, 0), draw.Over)
	return helm
}

func ExtractBody(skin image.Image) image.Image {
	body := image.NewNRGBA(image.Rect(0, 0, 16, 32))

	// Head
	draw.Draw(body, image.Rect(4, 0, 12, 8), skin, image.Pt(8, 8), draw.Src)
	hat := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	draw.Draw(hat, hat.Bounds(), skin, image.Pt(40, 8), draw.Src)
	draw.Draw(body, image.Rect(4, 0, 12, 8), hat, image.Pt(0, 0), draw.Over)

	// Torso
	draw.Draw(body, image.Rect(4, 8, 12, 20), skin, image.Pt(20, 20), draw.Src)

	// Arms
	draw.Draw(body, image.Rect(0, 8, 4, 20), skin, image.Pt(44, 20), draw.Src)   // right
	draw.Draw(body, image.Rect(12, 8, 16, 20), skin, image.Pt(36, 52), draw.Src) // left

	// Legs
	draw.Draw(body, image.Rect(4, 20, 8, 32), skin, image.Pt(4, 20), draw.Src)
	draw.Draw(body, image.Rect(8, 20, 12, 32), skin, image.Pt(20, 52), draw.Src)

	return body
}

func ExtractBust(skin image.Image) image.Image {
	bust := image.NewNRGBA(image.Rect(0, 0, 16, 16))

	// Head
	draw.Draw(bust, image.Rect(4, 0, 12, 8), skin, image.Pt(8, 8), draw.Src)
	hat := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	draw.Draw(hat, hat.Bounds(), skin, image.Pt(40, 8), draw.Src)
	draw.Draw(bust, image.Rect(4, 0, 12, 8), hat, image.Pt(0, 0), draw.Over)

	// Torso
	draw.Draw(bust, image.Rect(4, 8, 12, 16), skin, image.Pt(20, 20), draw.Src)

	// Arms
	draw.Draw(bust, image.Rect(0, 8, 4, 16), skin, image.Pt(44, 20), draw.Src)   // right
	draw.Draw(bust, image.Rect(12, 8, 16, 16), skin, image.Pt(36, 52), draw.Src) // left

	return bust
}

func ResizeNearest(src image.Image, width, height int) image.Image {
	if width <= 0 || height <= 0 {
		width, height = DefaultSize, DefaultSize
	}
	if width > MaxSize {
		width = MaxSize
	}
	if height > MaxSize {
		height = MaxSize
	}
	dst := image.NewNRGBA(image.Rect(0, 0, width, height))
	sb := src.Bounds()
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			dst.Set(x, y, src.At(sb.Min.X+x*sb.Dx()/width, sb.Min.Y+y*sb.Dy()/height))
		}
	}
	return dst
}

func ClampSize(s int) int {
	if s < MinSize {
		return DefaultSize
	}
	if s > MaxSize {
		return MaxSize
	}
	return s
}

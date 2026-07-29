// Генератор исходной иконки приложения: скруглённый квадрат с кольцом расхода.
package main

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

const (
	size    = 1024
	scale   = 4 // супер-сэмплинг
	inset   = 100.0
	corner  = 200.0
	ringPad = 210.0
	ringW   = 74.0
)

func main() {
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	center := float64(size) / 2
	radius := center - ringPad
	progress := 0.72

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var r, g, b, a float64

			for sy := 0; sy < scale; sy++ {
				for sx := 0; sx < scale; sx++ {
					px := float64(x) + (float64(sx)+0.5)/scale
					py := float64(y) + (float64(sy)+0.5)/scale

					sr, sg, sb, sa := sample(px, py, center, radius, progress)
					r += sr
					g += sg
					b += sb
					a += sa
				}
			}

			n := float64(scale * scale)
			img.Set(x, y, color.RGBA{
				R: uint8(r / n),
				G: uint8(g / n),
				B: uint8(b / n),
				A: uint8(a / n),
			})
		}
	}

	file, err := os.Create(os.Args[1])
	if err != nil {
		panic(err)
	}
	defer file.Close()

	if err := png.Encode(file, img); err != nil {
		panic(err)
	}
}

func sample(x, y, center, radius, progress float64) (r, g, b, a float64) {
	// Подложка — скруглённый квадрат в графитовом цвете.
	if !insideRoundedRect(x, y, inset, float64(size)-inset, corner) {
		return 0, 0, 0, 0
	}

	bgR, bgG, bgB := 28.0, 28.0, 32.0

	dx, dy := x-center, y-center
	dist := math.Hypot(dx, dy)

	if math.Abs(dist-radius) > ringW/2 {
		return bgR, bgG, bgB, 255
	}

	// Угол от 12 часов по часовой стрелке.
	angle := math.Atan2(dx, -dy)
	if angle < 0 {
		angle += 2 * math.Pi
	}
	fraction := angle / (2 * math.Pi)

	if fraction <= progress {
		return 52, 199, 89, 255
	}

	// Незаполненная часть кольца — приглушённая дорожка.
	return bgR + 40, bgG + 40, bgB + 42, 255
}

func insideRoundedRect(x, y, min, max, radius float64) bool {
	if x < min || x > max || y < min || y > max {
		return false
	}

	cx := math.Min(math.Max(x, min+radius), max-radius)
	cy := math.Min(math.Max(y, min+radius), max-radius)

	return math.Hypot(x-cx, y-cy) <= radius
}

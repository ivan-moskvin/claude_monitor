package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"math"
)

// Устройство принимает GIF размером 16, 32, 64 или 128 пикселей.
const panelSize = 128

// Индексы палитры. Нулевой индекс устройство показывает как чёрный, поэтому
// фон занимает именно его — иначе фоновый цвет пришлось бы отдельно защищать.
const (
	idxBackground uint8 = iota
	idxTrack
	idxGreen
	idxOrange
	idxRed
	idxCyan
	idxWhite
	idxGrey
)

var palette = color.Palette{
	color.RGBA{0x00, 0x00, 0x00, 0xff},
	color.RGBA{0x23, 0x23, 0x26, 0xff},
	color.RGBA{0x30, 0xd1, 0x58, 0xff},
	color.RGBA{0xff, 0x9f, 0x0a, 0xff},
	color.RGBA{0xff, 0x45, 0x3a, 0xff},
	color.RGBA{0x40, 0xc8, 0xe0, 0xff},
	color.RGBA{0xff, 0xff, 0xff, 0xff},
	color.RGBA{0x8e, 0x8e, 0x93, 0xff},
}

type panel struct {
	img *image.Paletted
}

func newPanel() *panel {
	return &panel{img: image.NewPaletted(image.Rect(0, 0, panelSize, panelSize), palette)}
}

func (p *panel) fillRect(x, y, w, h int, idx uint8) {
	for dy := 0; dy < h; dy++ {
		for dx := 0; dx < w; dx++ {
			px, py := x+dx, y+dy
			if px < 0 || py < 0 || px >= panelSize || py >= panelSize {
				continue
			}
			p.img.SetColorIndex(px, py, idx)
		}
	}
}

// drawArc рисует дугу окна: трек на весь круг и залитую часть от 12 часов
// по часовой стрелке. Сглаживания нет намеренно — на 128 пикселях оно
// превратилось бы в лишние цвета палитры, а края и так читаются.
func (p *panel) drawArc(radius, thickness int, fraction float64, fill, track uint8) {
	const center = panelSize / 2
	outer := float64(radius)
	inner := outer - float64(thickness)
	fraction = math.Max(0, math.Min(1, fraction))

	for y := 0; y < panelSize; y++ {
		for x := 0; x < panelSize; x++ {
			dx := float64(x) - center + 0.5
			dy := float64(y) - center + 0.5
			distance := math.Hypot(dx, dy)
			if distance > outer || distance < inner {
				continue
			}

			// Угол от вертикали вверх, по часовой стрелке: 0 → 1 за полный круг.
			angle := math.Atan2(dx, -dy)
			if angle < 0 {
				angle += 2 * math.Pi
			}
			position := angle / (2 * math.Pi)

			idx := track
			if position <= fraction {
				idx = fill
			}
			p.img.SetColorIndex(x, y, idx)
		}
	}
}

// encode отдаёт GIF и его хэш: хэш попадает в имя файла, поэтому устройство
// перекачивает картинку только когда она действительно изменилась.
func (p *panel) encode() ([]byte, string, error) {
	var buf bytes.Buffer
	if err := gif.Encode(&buf, p.img, nil); err != nil {
		return nil, "", fmt.Errorf("не удалось закодировать GIF: %w", err)
	}
	data := buf.Bytes()
	sum := sha256.Sum256(data)
	return data, hex.EncodeToString(sum[:8]), nil
}

// render собирает панель по снимку лимитов.
func render(state snapshot) ([]byte, string, error) {
	p := newPanel()

	five, week := state.window("five_hour"), state.window("seven_day")

	// Внешнее кольцо — пятичасовое окно, внутреннее — недельное: тот же
	// порядок, что в строке статуса, сначала то, что кончится раньше.
	p.drawArc(62, 9, five.fraction(), five.tint(idxGreen), idxTrack)
	p.drawArc(48, 7, week.fraction(), week.tint(idxCyan), idxTrack)

	if state.err != "" {
		p.drawTextCentered("NO", 52, 3, idxGrey)
		p.drawTextCentered("DATA", 76, 2, idxGrey)
		return p.encode()
	}

	p.drawTextCentered(five.percentLabel(), 44, 4, five.tint(idxWhite))
	p.drawTextCentered("7D "+week.percentLabel(), 78, 2, week.tint(idxGrey))

	switch {
	case five.expired:
		p.drawTextCentered("RESET", 96, 2, idxGrey)
	case state.stale:
		// Данные растут только в активной сессии: без неё цифры замирают,
		// и об этом надо сказать, иначе панель врёт молча.
		p.drawTextCentered("OLD "+countdownLabel(five.secondsLeft), 96, 2, idxGrey)
	default:
		p.drawTextCentered(countdownLabel(five.secondsLeft), 96, 2, idxCyan)
	}

	return p.encode()
}

// countdownLabel — «2:41» до сброса окна; часы не режем, минуты дополняем нулём.
func countdownLabel(seconds int) string {
	if seconds <= 0 {
		return "-:--"
	}
	minutes := seconds / 60
	return fmt.Sprintf("%d:%02d", minutes/60, minutes%60)
}

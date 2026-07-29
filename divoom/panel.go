package divoom

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"math"
	"time"

	"github.com/ivan-moskvin/claude_monitor/i18n"
)

// The device accepts a GIF sized 16, 32, 64 or 128 pixels.
const panelSize = 128

// Palette indices. The device shows index zero as black, so the background
// takes exactly that one — otherwise the background color would need protecting
// separately.
const (
	idxBackground uint8 = iota
	idxTrack
	idxGreen
	idxOrange
	idxRed
	idxCyan
	idxWhite
	idxGrey
	idxClaude
)

// The colors are the ones from the status line, out of the 256-color terminal
// palette: 35 green, 214 orange, 167 red, 38 cyan, 237 the background of the
// empty part, 250 the text on it. The panel and the status line show the same
// thing, and have no reason to differ in color.
var palette = color.Palette{
	color.RGBA{0x00, 0x00, 0x00, 0xff},
	color.RGBA{0x3a, 0x3a, 0x3a, 0xff},
	color.RGBA{0x00, 0xaf, 0x5f, 0xff},
	color.RGBA{0xff, 0xaf, 0x00, 0xff},
	color.RGBA{0xd7, 0x5f, 0x5f, 0xff},
	color.RGBA{0x00, 0xaf, 0xd7, 0xff},
	color.RGBA{0xff, 0xff, 0xff, 0xff},
	color.RGBA{0xbc, 0xbc, 0xbc, 0xff},
	color.RGBA{0xd9, 0x77, 0x57, 0xff},
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

func (p *panel) drawBar(x, y, w, h int, fraction float64, fill uint8, label string) {
	fraction = math.Max(0, math.Min(1, fraction))
	filled := int(math.Round(float64(w) * fraction))

	p.fillRect(x, y, w, h, idxTrack)
	p.fillRect(x, y, filled, h, fill)

	const labelScale = 2
	textX := x + (w-textWidth(label, labelScale))/2
	textY := y + (h-glyphHeight*labelScale)/2

	// On the filled background the label is dark, on the empty one light grey.
	// Red is the exception: it is dark enough for black digits to drown in it,
	// and the status line writes on it in white too.
	onFill := idxBackground
	if fill == idxRed {
		onFill = idxWhite
	}
	p.drawTextSplit(label, textX, textY, labelScale, x+filled, onFill, idxGrey)
}

// encode returns the GIF and its hash: the hash goes into the file name, so the
// device re-downloads the picture only when it really changed.
func (p *panel) encode() ([]byte, string, error) {
	var buf bytes.Buffer
	if err := gif.Encode(&buf, p.img, nil); err != nil {
		return nil, "", fmt.Errorf(i18n.T("could not encode the GIF: %w"), err)
	}
	data := buf.Bytes()
	sum := sha256.Sum256(data)
	return data, hex.EncodeToString(sum[:8]), nil
}

// Layout: the header with the Claude sparkle on top, three bars with their
// labels on the left below it.
const (
	labelX    = 6
	labelWide = 22
	barX      = 32
	barWidth  = panelSize - barX - labelX
	barHeight = 20
	rowGap    = 7
	headerY   = 8
	sparkSize = 17
	firstRowY = 38
	resetIcon = 15
)

func (p *panel) drawResetIcon(x, y, size int, idx uint8) {
	radius := float64(size) / 2
	center := radius - 0.5

	for dy := 0; dy < size; dy++ {
		for dx := 0; dx < size; dx++ {
			ox, oy := float64(dx)-center, float64(dy)-center
			distance := math.Hypot(ox, oy)
			if distance > radius || distance < radius-2.5 {
				continue
			}

			// The ring is broken at the upper right — that is where the arrow
			// head starts.
			angle := math.Atan2(ox, -oy)
			if angle > 0.2 && angle < 1.5 {
				continue
			}
			p.img.SetColorIndex(x+dx, y+dy, idx)
		}
	}

	// The arrow head at the break: three rows narrowing to the right.
	tipX, tipY := x+size/2, y
	for row := 0; row < 3; row++ {
		p.fillRect(tipX+row, tipY+row, 3-row, 1, idx)
		p.fillRect(tipX+row, tipY-row, 3-row, 1, idx)
	}
}

func (p *panel) drawSparkle(x, y, size int, idx uint8) {
	radius := size / 2

	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			ax, ay := math.Abs(float64(dx)), math.Abs(float64(dy))
			distance := math.Hypot(float64(dx), float64(dy))
			if distance > float64(radius) {
				continue
			}

			// A petal is the wider the closer it is to the center: the axial
			// rays have more room, the diagonal ones are half as long — that
			// keeps the sparkle from turning into an even star.
			axial := ax <= (float64(radius)-ay)/2.6 || ay <= (float64(radius)-ax)/2.6
			diagonal := math.Abs(ax-ay) <= (float64(radius)-math.Max(ax, ay))/2.6 &&
				distance <= float64(radius)*0.82

			if axial || diagonal {
				p.img.SetColorIndex(x+radius+dx, y+radius+dy, idx)
			}
		}
	}
}

func render(state snapshot) ([]byte, string, error) {
	p := newPanel()

	p.drawSparkle(labelX, headerY, sparkSize, idxClaude)
	p.drawText("CLAUDE", labelX+sparkSize+7, headerY+(sparkSize-glyphHeight*2)/2, 2, idxWhite)

	// The numbers only grow during an active Claude Code session: if the
	// snapshot has not been updated for a while, the percentages below describe
	// the past, and that has to be visible. Smaller than the header: otherwise
	// the mark stands right against it and reads as part of the word —
	// "CLAUDE2H".
	if state.stale {
		const ageScale = 1
		label := ageLabel(state.age)
		p.drawText(label, panelSize-labelX-textWidth(label, ageScale),
			headerY+(sparkSize-glyphHeight*ageScale)/2, ageScale, idxGrey)
	}

	if state.err != "" {
		p.drawTextCentered(i18n.T("NO"), 52, 3, idxGrey)
		p.drawTextCentered(i18n.T("DATA"), 86, 2, idxGrey)
		return p.encode()
	}

	five, week := state.window("five_hour"), state.window("seven_day")

	rowY := firstRowY
	p.drawText("5H", labelX, rowY+(barHeight-glyphHeight*2)/2, 2, idxGrey)
	p.drawBar(barX, rowY, barWidth, barHeight, five.fraction(), five.tint(), five.percentLabel())

	rowY += barHeight + rowGap
	p.drawResetIcon(labelX+(labelWide-resetIcon)/2, rowY+(barHeight-resetIcon)/2, resetIcon, idxGrey)
	p.drawBar(barX, rowY, barWidth, barHeight, five.elapsedFraction(), resetTint(five), resetLabel(five))

	rowY += barHeight + rowGap
	p.drawText("7D", labelX, rowY+(barHeight-glyphHeight*2)/2, 2, idxGrey)
	p.drawBar(barX, rowY, barWidth, barHeight, week.fraction(), week.tint(), week.percentLabel())

	return p.encode()
}

func resetLabel(five usageWindow) string {
	if five.expired {
		return i18n.T("RESET")
	}
	return countdownLabel(five.secondsLeft)
}

func resetTint(five usageWindow) uint8 {
	if five.expired {
		return idxGrey
	}
	return idxCyan
}

func ageLabel(age time.Duration) string {
	if hours := int(age.Hours()); hours > 0 {
		return fmt.Sprintf(i18n.T("%dH"), hours)
	}
	return fmt.Sprintf(i18n.T("%dM"), int(age.Minutes()))
}

// countdownLabel — "2:41" until the window resets. The status line spells the
// time with letters, but in a 5×7 grid the Russian letter for an hour is
// indistinguishable from a four, and the label reads as "24 41M". A colon can
// be mistaken for nothing and needs no translation.
func countdownLabel(seconds int) string {
	if seconds <= 0 {
		return "-"
	}
	minutes := seconds / 60
	return fmt.Sprintf("%d:%02d", minutes/60, minutes%60)
}

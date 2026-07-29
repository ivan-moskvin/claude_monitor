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
	"time"
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
	idxClaude
)

// Цвета — те же, что в строке статуса, из 256-цветной палитры терминала:
// 35 зелёный, 214 оранжевый, 167 красный, 38 циан, 237 фон пустой части,
// 250 текст на ней. Панель и строка статуса показывают одно и то же, и
// расходиться в цвете им незачем.
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

// drawBar рисует полосу расхода: фон на всю ширину, залитая часть цветом
// уровня, подпись по центру внутри. Та же схема, что в строке статуса —
// граница полосы читается без рамки, а число не уезжает от неё в сторону.
func (p *panel) drawBar(x, y, w, h int, fraction float64, fill uint8, label string) {
	fraction = math.Max(0, math.Min(1, fraction))
	filled := int(math.Round(float64(w) * fraction))

	p.fillRect(x, y, w, h, idxTrack)
	p.fillRect(x, y, filled, h, fill)

	const labelScale = 2
	textX := x + (w-textWidth(label, labelScale))/2
	textY := y + (h-glyphHeight*labelScale)/2

	// На залитом фоне подпись тёмная, на пустом — светло-серая. Исключение —
	// красный: он достаточно тёмный, чтобы чёрные цифры на нём тонули, и в
	// строке статуса на нём тоже пишут белым.
	onFill := idxBackground
	if fill == idxRed {
		onFill = idxWhite
	}
	p.drawTextSplit(label, textX, textY, labelScale, x+filled, onFill, idxGrey)
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

// Раскладка. Сверху заголовок с искрой Claude, ниже полосы: метки слева,
// полоса сброса — во всю ширину, у неё нет метки и время узнаётся по формату.
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

// drawResetIcon рисует круговую стрелку — метку строки со временем до сброса.
// Соседние строки подписаны словами, эта нет: «5H» и «7D» говорят про расход,
// а здесь другая величина, и её честнее обозначить знаком, чем ещё одной парой
// букв, которую пришлось бы расшифровывать.
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

			// Разрыв кольца справа сверху — там начинается наконечник.
			angle := math.Atan2(ox, -oy)
			if angle > 0.2 && angle < 1.5 {
				continue
			}
			p.img.SetColorIndex(x+dx, y+dy, idx)
		}
	}

	// Наконечник стрелки у разрыва: три ряда, сужающиеся вправо.
	tipX, tipY := x+size/2, y
	for row := 0; row < 3; row++ {
		p.fillRect(tipX+row, tipY+row, 3-row, 1, idx)
		p.fillRect(tipX+row, tipY-row, 3-row, 1, idx)
	}
}

// drawSparkle рисует искру Claude: четыре длинных луча по осям и четыре
// коротких по диагоналям, сужающиеся к концам.
func (p *panel) drawSparkle(x, y, size int, idx uint8) {
	radius := size / 2

	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			ax, ay := math.Abs(float64(dx)), math.Abs(float64(dy))
			distance := math.Hypot(float64(dx), float64(dy))
			if distance > float64(radius) {
				continue
			}

			// Лепесток тем шире, чем ближе к центру: у осевых лучей запас
			// больше, диагональные вдвое короче — так искра не превращается
			// в равномерную звезду.
			axial := ax <= (float64(radius)-ay)/2.6 || ay <= (float64(radius)-ax)/2.6
			diagonal := math.Abs(ax-ay) <= (float64(radius)-math.Max(ax, ay))/2.6 &&
				distance <= float64(radius)*0.82

			if axial || diagonal {
				p.img.SetColorIndex(x+radius+dx, y+radius+dy, idx)
			}
		}
	}
}

// render собирает панель по снимку лимитов — тремя полосами, как в строке
// статуса: сначала то, что кончится раньше, недельное окно в конце.
func render(state snapshot) ([]byte, string, error) {
	p := newPanel()

	p.drawSparkle(labelX, headerY, sparkSize, idxClaude)
	p.drawText("CLAUDE", labelX+sparkSize+7, headerY+(sparkSize-glyphHeight*2)/2, 2, idxWhite)

	// Данные растут только в активной сессии Claude Code: если снапшот давно
	// не обновлялся, проценты ниже описывают прошлое, и это должно быть видно.
	if state.stale {
		label := ageLabel(state.age)
		p.drawText(label, panelSize-labelX-textWidth(label, 2), headerY+(sparkSize-glyphHeight*2)/2, 2, idxGrey)
	}

	if state.err != "" {
		p.drawTextCentered("НЕТ", 52, 3, idxGrey)
		p.drawTextCentered("ДАННЫХ", 86, 2, idxGrey)
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

// resetLabel — что писать в полосе сброса. Возраст снапшота сюда не
// вмешивается: время до сброса считается от абсолютной метки resets_at
// и тикает верно, даже когда расход давно не обновлялся.
func resetLabel(five usageWindow) string {
	if five.expired {
		return "СБРОС"
	}
	return countdownLabel(five.secondsLeft)
}

func resetTint(five usageWindow) uint8 {
	if five.expired {
		return idxGrey
	}
	return idxCyan
}

// ageLabel — сколько снапшот не обновлялся. Проценты расхода за это время
// могли вырасти, поэтому метка стоит у заголовка, а не у цифр окна.
func ageLabel(age time.Duration) string {
	if hours := int(age.Hours()); hours > 0 {
		return fmt.Sprintf("%dЧ", hours)
	}
	return fmt.Sprintf("%dМ", int(age.Minutes()))
}

// countdownLabel — «2:41» до сброса окна. В строке статуса время подписано
// буквами, но в шрифте 5×7 «Ч» неотличима от четвёрки: «2Ч 41М» читается
// как «24 41М». Двоеточие спутать не с чем.
func countdownLabel(seconds int) string {
	if seconds <= 0 {
		return "-"
	}
	minutes := seconds / 60
	return fmt.Sprintf("%d:%02d", minutes/60, minutes%60)
}

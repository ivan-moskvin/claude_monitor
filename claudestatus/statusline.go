package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"time"
)

const (
	barWidth        = 10
	fiveHourSeconds = 5 * 60 * 60
)

// Цвета 256-цветной палитры. Полосы расхода красятся по порогам, полоса
// сброса всегда циан — там время, а не риск.
const (
	colorGreen     = 35
	colorOrange    = 214
	colorRed       = 167
	colorCyan      = 38
	colorUpdate    = 178
	colorEmptyBG   = 237
	colorEmptyFG   = 250
	colorDarkText  = 16
	colorLightText = 231
)

// Шкала уровня reasoning effort: фазы круга — те же символы, что и у самого
// Claude Code. Цвет намеренно в фиолетово-розовой гамме, чтобы не путался
// с зелёным/оранжевым/красным полос расхода.
var effortStyles = map[string]struct {
	mark  string
	color int
}{
	"low":    {"○", 245},
	"medium": {"◔", 111},
	"high":   {"◑", 141},
	"xhigh":  {"◕", 171},
	"max":    {"●", 199},
}

type sessionInput struct {
	RateLimits map[string]json.RawMessage `json:"rate_limits"`
	Model      struct {
		DisplayName string `json:"display_name"`
	} `json:"model"`
	// Приходит только когда модель поддерживает параметр effort.
	Effort struct {
		Level string `json:"level"`
	} `json:"effort"`
}

type window struct {
	UsedPercentage *float64 `json:"used_percentage"`
	ResetsAt       *float64 `json:"resets_at"`
}

// statusline печатает строку по JSON сессии со stdin — это режим по умолчанию,
// именно он прописан в settings.json.
func statusline() {
	// Проверку обновлений заводим в любом случае: даже пустой вход означает,
	// что Claude Code жив и строку кто-то видит.
	defer autoCheck()

	var input sessionInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		fmt.Println("—")
		return
	}

	var parts []string

	if name := input.Model.DisplayName; name != "" {
		// Уровень показываем рядом с моделью: заполненность круга читается
		// и без цвета, а места занимает один символ вместо слова.
		if style, ok := effortStyles[input.Effort.Level]; ok {
			name = colorized(name+" "+style.mark, style.color)
		}
		parts = append(parts, name)
	}

	five := parseWindow(input.RateLimits["five_hour"])

	if five.UsedPercentage != nil {
		used := *five.UsedPercentage
		parts = append(parts, fmt.Sprintf("5h %s", labeledBar(used, percentLabel(used), usageColor(used))))
	}

	if left, ok := secondsLeft(five.ResetsAt); ok {
		elapsed := float64(fiveHourSeconds-left) / fiveHourSeconds * 100
		parts = append(parts, fmt.Sprintf("сброс %s", labeledBar(elapsed, countdown(left), colorCyan)))
	}

	// Недельное окно уходит в конец: оно меняется медленно и мешает
	// читать то, что важно прямо сейчас.
	if week := parseWindow(input.RateLimits["seven_day"]); week.UsedPercentage != nil {
		used := *week.UsedPercentage
		parts = append(parts, fmt.Sprintf("7d %s", labeledBar(used, percentLabel(used), usageColor(used))))
	}

	// Значок обновления — в самом конце: он не про текущую сессию и не должен
	// сдвигать цифры лимитов, к месту которых глаз уже привык.
	if tag, ok := updateAvailable(); ok {
		parts = append(parts, colorized("↑ "+tag, colorUpdate))
	}

	if len(parts) == 0 {
		fmt.Println("—")
		return
	}
	fmt.Println(strings.Join(parts, " · "))
}

// labeledBar рисует полосу, внутри которой по центру написан label. Фон идёт
// на всю ширину — залитая часть цветом уровня, остаток тёмно-серым, — поэтому
// граница полосы читается без рамки.
func labeledBar(percentage float64, label string, color int) string {
	runes := []rune(label)
	if len(runes) > barWidth {
		runes = runes[:barWidth]
	}

	// Подпись нечётной длины ровно по центру не встаёт — остаётся лишний пробел.
	// Кладём его слева: иначе «12%» и «39м» заметно съезжают к левому краю полосы.
	pad := barWidth - len(runes)
	left := (pad + 1) / 2
	text := []rune(strings.Repeat(" ", left) + string(runes) + strings.Repeat(" ", pad-left))

	filled := int(math.RoundToEven(percentage / 100 * barWidth))
	filled = max(0, min(barWidth, filled))

	textColor := colorDarkText
	if color == colorRed {
		textColor = colorLightText
	}

	return fmt.Sprintf("\x1b[1;48;5;%d;38;5;%dm%s\x1b[0;48;5;%d;38;5;%dm%s\x1b[0m",
		color, textColor, string(text[:filled]),
		colorEmptyBG, colorEmptyFG, string(text[filled:]))
}

func usageColor(percentage float64) int {
	switch {
	case percentage >= 85:
		return colorRed
	case percentage >= 60:
		return colorOrange
	default:
		return colorGreen
	}
}

func colorized(text string, color int) string {
	return fmt.Sprintf("\x1b[1;38;5;%dm%s\x1b[0m", color, text)
}

// percentLabel не выравнивает число по правому краю: ведущие пробелы попали бы
// внутрь подписи и сдвинули её вправо от центра полосы.
func percentLabel(percentage float64) string {
	return fmt.Sprintf("%.0f%%", percentage)
}

func secondsLeft(resetsAt *float64) (int, bool) {
	if resetsAt == nil {
		return 0, false
	}
	// Текущее время берём дробным, иначе остаток округляется вверх
	// и обратный отсчёт показывает лишнюю минуту.
	now := float64(time.Now().UnixNano()) / 1e9
	seconds := int(normalizeEpoch(*resetsAt) - now)
	if seconds <= 0 {
		return 0, false
	}
	return seconds, true
}

// countdown пишет часы даже нулевые: без них подпись на переходе через час
// сжимается с шести символов до трёх и цифры прыгают к центру полосы.
func countdown(seconds int) string {
	minutes := seconds / 60
	hours, minutes := minutes/60, minutes%60
	return fmt.Sprintf("%dч %02dм", hours, minutes)
}

// normalizeEpoch принимает Unix-время в секундах или миллисекундах.
func normalizeEpoch(value float64) float64 {
	if value > 1e12 {
		return value / 1000
	}
	return value
}

func parseWindow(raw json.RawMessage) window {
	var w window
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &w)
	}
	return w
}

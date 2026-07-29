// Команда claude-statusline — writer в связке claude_monitor.
//
// Claude Code вызывает её как statusLine-команду: подаёт JSON сессии на stdin,
// забирает строку статуса из stdout. Попутно команда сохраняет лимиты
// в ~/.claude/usage-snapshot.json — единственный контракт с виджетом.
package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
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
	colorEmptyBG   = 237
	colorEmptyFG   = 250
	colorDarkText  = 16
	colorLightText = 231
)

// Шкала уровня reasoning effort: намеренно в фиолетово-розовой гамме,
// чтобы не путалась с зелёным/оранжевым/красным полос расхода.
var effortColors = map[string]int{
	"low":    245,
	"medium": 111,
	"high":   141,
	"xhigh":  171,
	"max":    199,
}

// config — настройки виджета. Файла может не быть: по умолчанию строка
// чёрно-белая, цвет включается тумблером в попапе.
type config struct {
	StatuslineColor bool `json:"statusline_color"`
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

type snapshot struct {
	RateLimits map[string]json.RawMessage `json:"rate_limits"`
	UpdatedAt  string                     `json:"updated_at"`
}

func main() {
	var input sessionInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		fmt.Println("—")
		return
	}

	if len(input.RateLimits) > 0 {
		// Ошибку записи глотаем намеренно: строка статуса важнее снапшота
		// и не должна пропадать из-за проблем с диском.
		_ = writeSnapshot(snapshot{
			RateLimits: input.RateLimits,
			UpdatedAt:  time.Now().UTC().Format("2006-01-02T15:04:05.000000-07:00"),
		})
	}

	colorful := readConfig().StatuslineColor

	var parts []string

	if name := input.Model.DisplayName; name != "" {
		if color, ok := effortColors[input.Effort.Level]; ok && colorful {
			name = colorized(name, color)
		}
		parts = append(parts, name)
	}

	five := parseWindow(input.RateLimits["five_hour"])

	if five.UsedPercentage != nil {
		used := *five.UsedPercentage
		parts = append(parts, fmt.Sprintf("5h %s", labeledBar(used, percentLabel(used), usageColor(used), colorful)))
	}

	if week := parseWindow(input.RateLimits["seven_day"]); week.UsedPercentage != nil {
		used := *week.UsedPercentage
		parts = append(parts, fmt.Sprintf("7d %s", labeledBar(used, percentLabel(used), usageColor(used), colorful)))
	}

	if left, ok := secondsLeft(five.ResetsAt); ok {
		elapsed := float64(fiveHourSeconds-left) / fiveHourSeconds * 100
		parts = append(parts, fmt.Sprintf("сброс %s", labeledBar(elapsed, countdown(left), colorCyan, colorful)))
	}

	if len(parts) == 0 {
		fmt.Println("—")
		return
	}
	fmt.Println(strings.Join(parts, " · "))
}

// labeledBar рисует полосу, внутри которой по центру написан label.
//
// В цветном режиме фон идёт на всю ширину — залитая часть цветом уровня,
// остаток тёмно-серым, — поэтому граница полосы читается сама. В чёрно-белом
// цвета нет вовсе, поэтому заливка показана инверсией, а края — рамкой.
func labeledBar(percentage float64, label string, color int, colorful bool) string {
	runes := []rune(label)
	if len(runes) > barWidth {
		runes = runes[:barWidth]
	}

	pad := barWidth - len(runes)
	text := []rune(strings.Repeat(" ", pad/2) + string(runes) + strings.Repeat(" ", pad-pad/2))

	filled := int(math.RoundToEven(percentage / 100 * barWidth))
	filled = max(0, min(barWidth, filled))

	if !colorful {
		body := fmt.Sprintf("\x1b[7m%s\x1b[27m\x1b[2m%s\x1b[22m",
			string(text[:filled]), string(text[filled:]))
		return fmt.Sprintf("\x1b[2m▕\x1b[22m%s\x1b[2m▏\x1b[22m", body)
	}

	textColor := colorDarkText
	if color == colorRed {
		textColor = colorLightText
	}

	return fmt.Sprintf("\x1b[1;48;5;%d;38;5;%dm%s\x1b[0;48;5;%d;38;5;%dm%s\x1b[0m",
		color, textColor, string(text[:filled]),
		colorEmptyBG, colorEmptyFG, string(text[filled:]))
}

// readConfig читает настройки виджета. Любая проблема с файлом — не повод
// ломать строку статуса: возвращаем значения по умолчанию.
func readConfig() config {
	var cfg config

	home, err := os.UserHomeDir()
	if err != nil {
		return cfg
	}

	data, err := os.ReadFile(filepath.Join(home, ".claude", "claude-monitor.json"))
	if err != nil {
		return cfg
	}

	_ = json.Unmarshal(data, &cfg)
	return cfg
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

// percentLabel держит ширину подписи фиксированной, иначе текст внутри полосы
// прыгает при переходе 3% → 20% → 100%.
func percentLabel(percentage float64) string {
	return fmt.Sprintf("%3.0f%%", percentage)
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

func countdown(seconds int) string {
	minutes := seconds / 60
	hours, minutes := minutes/60, minutes%60
	if hours > 0 {
		return fmt.Sprintf("%dч %02dм", hours, minutes)
	}
	return fmt.Sprintf("%dм", minutes)
}

// normalizeEpoch принимает Unix-время в секундах или миллисекундах —
// та же эвристика, что и у читателя снапшота.
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

func writeSnapshot(payload snapshot) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	dir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	// Пишем через временный файл: виджет читает снапшот параллельно
	// и не должен наткнуться на половину записи.
	tmp := filepath.Join(dir, "usage-snapshot.tmp")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, "usage-snapshot.json"))
}

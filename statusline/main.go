// Команда claude-statusline — строка статуса с лимитами Claude.
//
// Claude Code вызывает её как statusLine-команду: подаёт JSON сессии на stdin,
// забирает строку статуса из stdout. С ключом --install команда прописывает
// себя в ~/.claude/settings.json.
package main

import (
	"bytes"
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
var effortStyles = map[string]struct {
	label string
	color int
}{
	"low":    {"Low", 245},
	"medium": {"Medium", 111},
	"high":   {"High", 141},
	"xhigh":  {"xHigh", 171},
	"max":    {"Max", 199},
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

func main() {
	if len(os.Args) > 1 {
		if os.Args[1] != "--install" {
			fmt.Fprintf(os.Stderr, "Неизвестный аргумент: %s (есть только --install)\n", os.Args[1])
			os.Exit(2)
		}
		if err := install(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	var input sessionInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		fmt.Println("—")
		return
	}

	var parts []string

	if name := input.Model.DisplayName; name != "" {
		// Уровень пишем рядом с моделью: цвет отличает уровни друг от друга,
		// но не говорит, какой именно сейчас включён.
		if style, ok := effortStyles[input.Effort.Level]; ok {
			name = colorized(name+" "+style.label, style.color)
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

	pad := barWidth - len(runes)
	text := []rune(strings.Repeat(" ", pad/2) + string(runes) + strings.Repeat(" ", pad-pad/2))

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

func countdown(seconds int) string {
	minutes := seconds / 60
	hours, minutes := minutes/60, minutes%60
	if hours > 0 {
		return fmt.Sprintf("%dч %02dм", hours, minutes)
	}
	return fmt.Sprintf("%dм", minutes)
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

// install прописывает эту команду в statusLine ~/.claude/settings.json.
// Путь берём абсолютный: строка статуса вызывается из любого каталога.
func install() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("не удалось определить свой путь: %w", err)
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return fmt.Errorf("не удалось определить свой путь: %w", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".claude")
	path := filepath.Join(dir, "settings.json")

	settings := map[string]any{}
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if len(bytes.TrimSpace(data)) > 0 {
			if json.Unmarshal(data, &settings) != nil {
				return fmt.Errorf("%s не разбирается — поправьте его вручную", path)
			}
		}
		// Бэкап до записи и только при существующем файле —
		// иначе затрём чужие настройки без возможности откатиться.
		if err := os.WriteFile(path+".bak", data, 0o644); err != nil {
			return fmt.Errorf("не удалось сделать бэкап settings.json: %w", err)
		}
	case !os.IsNotExist(err):
		return fmt.Errorf("не удалось прочитать settings.json: %w", err)
	}

	// Кавычки — на случай пробелов в пути к репозиторию.
	command := fmt.Sprintf("%q", exe)
	if previous, ok := settings["statusLine"].(map[string]any); ok {
		if was, _ := previous["command"].(string); was != "" && was != command {
			fmt.Printf("Заменяю прежнюю строку статуса: %s\n", was)
			fmt.Printf("Прежние настройки — в %s.bak\n", path)
		}
	}

	settings["statusLine"] = map[string]string{"type": "command", "command": command}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("не удалось создать %s: %w", dir, err)
	}

	updated, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(updated, '\n'), 0o644)
}

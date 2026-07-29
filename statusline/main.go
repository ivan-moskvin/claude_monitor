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

type sessionInput struct {
	RateLimits map[string]json.RawMessage `json:"rate_limits"`
	Model      struct {
		DisplayName string `json:"display_name"`
	} `json:"model"`
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

	var parts []string

	if name := input.Model.DisplayName; name != "" {
		parts = append(parts, name)
	}

	five := parseWindow(input.RateLimits["five_hour"])

	if five.UsedPercentage != nil {
		used := *five.UsedPercentage
		parts = append(parts, fmt.Sprintf("5h %s", labeledBar(used, fmt.Sprintf("%.0f%%", used))))
	}

	if left, ok := secondsLeft(five.ResetsAt); ok {
		elapsed := float64(fiveHourSeconds-left) / fiveHourSeconds * 100
		parts = append(parts, fmt.Sprintf("сброс %s", labeledBar(elapsed, countdown(left))))
	}

	if len(parts) == 0 {
		fmt.Println("—")
		return
	}
	fmt.Println(strings.Join(parts, " · "))
}

// labeledBar рисует полосу в рамке, внутри которой по центру написан label:
// заполненная часть выводится инверсией, незаполненная — приглушённой.
func labeledBar(percentage float64, label string) string {
	runes := []rune(label)
	if len(runes) > barWidth {
		runes = runes[:barWidth]
	}

	pad := barWidth - len(runes)
	text := []rune(strings.Repeat(" ", pad/2) + string(runes) + strings.Repeat(" ", pad-pad/2))

	filled := int(math.RoundToEven(percentage / 100 * barWidth))
	filled = max(0, min(barWidth, filled))

	body := fmt.Sprintf("\x1b[7m%s\x1b[27m\x1b[2m%s\x1b[22m", string(text[:filled]), string(text[filled:]))
	return fmt.Sprintf("\x1b[2m▕\x1b[22m%s\x1b[2m▏\x1b[22m", body)
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

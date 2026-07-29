package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Снапшот старше этого возраста описывает прошлое: Claude Code обновляет его
// только во время сессии.
const staleAfter = 90 * time.Second

type snapshot struct {
	windows map[string]usageWindow
	stale   bool
	// Возраст снапшота; показываем его, когда данные перестали обновляться.
	age time.Duration
	err string
}

type usageWindow struct {
	used        float64
	secondsLeft int
	expired     bool
	present     bool
}

type rawSnapshot struct {
	RateLimits map[string]struct {
		UsedPercentage *float64 `json:"used_percentage"`
		ResetsAt       *float64 `json:"resets_at"`
	} `json:"rate_limits"`
	UpdatedAt string `json:"updated_at"`
}

func snapshotPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "usage-snapshot.json"), nil
}

func readSnapshot() snapshot {
	path, err := snapshotPath()
	if err != nil {
		return snapshot{err: err.Error()}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return snapshot{err: "снапшот не найден"}
	}

	var raw rawSnapshot
	if err := json.Unmarshal(data, &raw); err != nil {
		return snapshot{err: "снапшот повреждён"}
	}

	now := float64(time.Now().UnixNano()) / 1e9
	windows := make(map[string]usageWindow, len(raw.RateLimits))
	for key, entry := range raw.RateLimits {
		if entry.UsedPercentage == nil {
			continue
		}
		w := usageWindow{used: *entry.UsedPercentage, present: true}
		if entry.ResetsAt != nil {
			resetsAt := normalizeEpoch(*entry.ResetsAt)
			if resetsAt <= now {
				// Проценты описывают прошедшее окно, в новом расход
				// начинается с нуля — старое число показывать нельзя.
				w.expired = true
				w.used = 0
			} else {
				w.secondsLeft = int(resetsAt - now)
			}
		}
		windows[key] = w
	}

	if len(windows) == 0 {
		return snapshot{err: "лимитов в снапшоте нет"}
	}

	result := snapshot{windows: windows}
	if updated, err := time.Parse(time.RFC3339, raw.UpdatedAt); err == nil {
		result.age = time.Since(updated)
		result.stale = result.age >= staleAfter
	}
	return result
}

func (s snapshot) window(id string) usageWindow {
	return s.windows[id]
}

// Длина пятичасового окна — по ней считается, сколько его уже прошло.
const fiveHourSeconds = 5 * 60 * 60

func (w usageWindow) fraction() float64 {
	return w.used / 100
}

// elapsedFraction — какая часть окна прожита. Полоса сброса показывает
// именно время, а не расход: в строке статуса это отдельная шкала.
func (w usageWindow) elapsedFraction() float64 {
	if w.expired || w.secondsLeft <= 0 {
		return 1
	}
	return float64(fiveHourSeconds-w.secondsLeft) / fiveHourSeconds
}

func (w usageWindow) percentLabel() string {
	if !w.present {
		return "-"
	}
	return fmt.Sprintf("%.0f%%", w.used)
}

// tint — единственное место с порогами цвета: до 60% базовый цвет окна,
// до 85% оранжевый, дальше красный. Сброшенное окно гасим.
func (w usageWindow) tint(base uint8) uint8 {
	switch {
	case !w.present || w.expired:
		return idxGrey
	case w.used >= 85:
		return idxRed
	case w.used >= 60:
		return idxOrange
	default:
		return base
	}
}

// normalizeEpoch принимает Unix-время в секундах или миллисекундах.
func normalizeEpoch(value float64) float64 {
	if value > 1e12 {
		return value / 1000
	}
	return value
}

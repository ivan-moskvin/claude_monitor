package divoom

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ivan-moskvin/claude_monitor/i18n"
	"github.com/ivan-moskvin/claude_monitor/paths"
)

// A snapshot older than this describes the past: Claude Code only refreshes it
// during a session.
const staleAfter = 90 * time.Second

const snapshotName = "usage-snapshot.json"

type snapshot struct {
	windows map[string]usageWindow
	stale   bool
	// The age of the snapshot; shown once the data stops being refreshed.
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
	return paths.File(snapshotName)
}

func readSnapshot() snapshot {
	path, err := snapshotPath()
	if err != nil {
		return snapshot{err: err.Error()}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return snapshot{err: i18n.T("no snapshot found")}
	}

	var raw rawSnapshot
	if err := json.Unmarshal(data, &raw); err != nil {
		return snapshot{err: i18n.T("the snapshot is damaged")}
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
				// The percentages describe a window that is over; in the new
				// one usage starts from zero — the old number must not be
				// shown.
				w.expired = true
				w.used = 0
			} else {
				w.secondsLeft = int(resetsAt - now)
			}
		}
		windows[key] = w
	}

	if len(windows) == 0 {
		return snapshot{err: i18n.T("the snapshot holds no limits")}
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

// usageKey — the percentages only, without the countdown: their growth is shown
// at once, a shifted minute can wait (see minSendInterval).
func (s snapshot) usageKey() string {
	if s.err != "" {
		return "err:" + s.err
	}
	var key strings.Builder
	for _, id := range []string{"five_hour", "seven_day", "seven_day_opus"} {
		w := s.windows[id]
		fmt.Fprintf(&key, "%s=%.0f/%t;", id, w.used, w.expired)
	}
	return key.String()
}

// The length of the five-hour window — how much of it has passed is counted
// against this.
const fiveHourSeconds = 5 * 60 * 60

func (w usageWindow) fraction() float64 {
	return w.used / 100
}

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

// tint — the only place with the color thresholds: green up to 60%, orange up
// to 85%, red above. Exactly like usageColor in the status line, and both usage
// windows are painted the same way: cyan belongs to the reset bar alone,
// because that one shows time and not risk. A window that has reset is dimmed.
func (w usageWindow) tint() uint8 {
	switch {
	case !w.present || w.expired:
		return idxGrey
	case w.used >= 85:
		return idxRed
	case w.used >= 60:
		return idxOrange
	default:
		return idxGreen
	}
}

// normalizeEpoch accepts Unix time in seconds or in milliseconds.
func normalizeEpoch(value float64) float64 {
	if value > 1e12 {
		return value / 1000
	}
	return value
}

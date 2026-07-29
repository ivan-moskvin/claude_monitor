package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/ivan-moskvin/claude_monitor/divoom"
	"github.com/ivan-moskvin/claude_monitor/i18n"
)

const (
	barWidth        = 10
	fiveHourSeconds = 5 * 60 * 60
)

// Colors of the 256-color palette. Usage bars are painted by threshold, the
// reset bar is always cyan — it shows time, not risk.
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

// The reasoning effort scale: the circle phases are the very symbols Claude
// Code itself uses. The color is deliberately purple to pink so that it is
// never confused with the green/orange/red of the usage bars.
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
	// Present only when the model supports the effort parameter.
	Effort struct {
		Level string `json:"level"`
	} `json:"effort"`
}

type window struct {
	UsedPercentage *float64 `json:"used_percentage"`
	ResetsAt       *float64 `json:"resets_at"`
}

// statusline prints the line from the session JSON on stdin — the default mode,
// and the one settings.json points at.
func statusline() {
	// The update check is started in any case: even empty input means Claude
	// Code is alive and somebody is looking at the line.
	defer autoCheck()

	var input sessionInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		fmt.Println("—")
		return
	}

	// The limits arrive here and only during a session: there is nowhere else
	// to get them. The snapshot is the only way to show them outside the status
	// line; a write error is swallowed, the status line matters more.
	_ = saveSnapshot(input.RateLimits)

	// The Divoom panel lives only while the bridge is running. Start it from
	// here: the check takes milliseconds and does nothing without a Times Gate
	// on the network.
	divoom.EnsureRunning()

	var parts []string

	if name := input.Model.DisplayName; name != "" {
		// The level goes next to the model: how full the circle is reads even
		// without color, and it costs one character instead of a word.
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
		parts = append(parts, fmt.Sprintf("%s %s", i18n.T("reset"), labeledBar(elapsed, countdown(left), colorCyan)))
	}

	// The weekly window goes last: it moves slowly and gets in the way of what
	// matters right now.
	if week := parseWindow(input.RateLimits["seven_day"]); week.UsedPercentage != nil {
		used := *week.UsedPercentage
		parts = append(parts, fmt.Sprintf("7d %s", labeledBar(used, percentLabel(used), usageColor(used))))
	}

	// The update mark comes at the very end: it is not about the current
	// session and must not shift the limit numbers the eye is already used to.
	if tag, ok := updateAvailable(); ok {
		parts = append(parts, colorized("↑ "+tag, colorUpdate))
	}

	if len(parts) == 0 {
		fmt.Println("—")
		return
	}
	fmt.Println(strings.Join(parts, " · "))
}

// labeledBar draws a bar with label centered inside it. The background spans
// the full width — the filled part in the level color, the rest dark grey — so
// the edge of the bar reads without a frame.
func labeledBar(percentage float64, label string, color int) string {
	runes := []rune(label)
	if len(runes) > barWidth {
		runes = runes[:barWidth]
	}

	// A label of odd length does not center exactly — one space is left over.
	// Put it on the left: otherwise "12%" and "39m" drift noticeably towards
	// the left edge of the bar.
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

// percentLabel does not right-align the number: leading spaces would end up
// inside the label and push it right of the bar center.
func percentLabel(percentage float64) string {
	return fmt.Sprintf("%.0f%%", percentage)
}

func secondsLeft(resetsAt *float64) (int, bool) {
	if resetsAt == nil {
		return 0, false
	}
	// Take the current time as a fraction, otherwise the remainder rounds up
	// and the countdown shows one minute too many.
	now := float64(time.Now().UnixNano()) / 1e9
	seconds := int(normalizeEpoch(*resetsAt) - now)
	if seconds <= 0 {
		return 0, false
	}
	return seconds, true
}

// countdown prints the hours even when they are zero: without them the label
// shrinks from six characters to three as it crosses an hour, and the digits
// jump towards the center of the bar.
func countdown(seconds int) string {
	minutes := seconds / 60
	hours, minutes := minutes/60, minutes%60
	return fmt.Sprintf(i18n.T("%dh %02dm"), hours, minutes)
}

// normalizeEpoch accepts Unix time in seconds or in milliseconds.
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

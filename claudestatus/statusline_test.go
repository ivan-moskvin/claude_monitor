package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ivan-moskvin/claudestatus/internal/testenv"
	"github.com/ivan-moskvin/claudestatus/paths"
)

// The line is never allowed to disappear: whatever arrives on stdin, Claude
// Code gets a line back.
func TestStatuslineAlwaysPrintsALine(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"nothing at all", ""},
		{"not JSON", "not json"},
		{"half a message", `{"rate_limits":`},
		{"an empty object", "{}"},
		{"no limits", `{"rate_limits":{}}`},
		{"a window with no numbers", `{"rate_limits":{"five_hour":{}}}`},
		{"a window that does not parse", `{"rate_limits":{"five_hour":"who knows"}}`},
		{"null limits", `{"rate_limits":null}`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := runStatusline(t, c.input); got != "—" {
				t.Errorf("statusline printed %q; want the dash", got)
			}
		})
	}
}

func TestStatuslineShowsWhatItWasGiven(t *testing.T) {
	line := runStatusline(t, fmt.Sprintf(`{
		"model": {"display_name": "Opus 5"},
		"effort": {"level": "high"},
		"rate_limits": {
			"five_hour": {"used_percentage": 42, "resets_at": %d},
			"seven_day": {"used_percentage": 13}
		}
	}`, resetsIn(2*time.Hour+30*time.Minute+30*time.Second)))

	parts := strings.Split(line, " · ")
	if len(parts) != 4 {
		t.Fatalf("the line has %d parts: %q", len(parts), line)
	}

	// The model comes first, with the effort circle right next to it.
	if !strings.Contains(parts[0], "Opus 5") || !strings.Contains(parts[0], "◑") {
		t.Errorf("the model part is %q; want the name and the effort mark", visible(parts[0]))
	}
	// Then the five-hour window, the reset countdown, and the weekly window last.
	if !strings.HasPrefix(parts[1], "5h ") || !strings.Contains(parts[1], "42%") {
		t.Errorf("the 5h part is %q", visible(parts[1]))
	}
	if !strings.Contains(parts[2], "2") || !strings.Contains(parts[2], "30") {
		t.Errorf("the reset part is %q; want 2 hours 30 minutes left", visible(parts[2]))
	}
	if !strings.HasPrefix(parts[3], "7d ") || !strings.Contains(parts[3], "13%") {
		t.Errorf("the 7d part is %q", visible(parts[3]))
	}
}

// The reset bar is the only cyan one — that is how the test finds it whatever
// language the words are in.
const resetBarColor = "48;5;38"

// A window that has already reset is not drawn at all: Claude Code sends the
// new numbers by itself, and a countdown to a moment in the past means nothing.
func TestStatuslineDropsAWindowThatReset(t *testing.T) {
	line := runStatusline(t, fmt.Sprintf(
		`{"rate_limits":{"five_hour":{"used_percentage":42,"resets_at":%d}}}`,
		time.Now().Add(-time.Minute).Unix()))

	if strings.Contains(line, resetBarColor) {
		t.Errorf("the line still counts down to a window that is over: %q", visible(line))
	}
	// The usage itself is still shown: Claude Code will redraw it in a moment.
	if !strings.Contains(line, "42%") {
		t.Errorf("the usage disappeared along with the countdown: %q", visible(line))
	}
}

func TestStatuslineWithoutAResetTime(t *testing.T) {
	line := runStatusline(t, `{"rate_limits":{"five_hour":{"used_percentage":42}}}`)

	if strings.Contains(line, resetBarColor) {
		t.Errorf("a countdown appeared out of nowhere: %q", visible(line))
	}
}

func TestStatuslineEffortMark(t *testing.T) {
	cases := []struct {
		level string
		mark  string
	}{
		{"low", "○"},
		{"medium", "◔"},
		{"high", "◑"},
		{"xhigh", "◕"},
		{"max", "●"},
	}

	for _, c := range cases {
		t.Run(c.level, func(t *testing.T) {
			line := runStatusline(t, fmt.Sprintf(
				`{"model":{"display_name":"Opus 5"},"effort":{"level":%q}}`, c.level))
			if !strings.Contains(line, c.mark) {
				t.Errorf("level %q drew %q; want %q in it", c.level, visible(line), c.mark)
			}
		})
	}

	// A model without the effort parameter, or one whose level we do not know,
	// is printed as it came — no marks invented.
	for _, input := range []string{
		`{"model":{"display_name":"Opus 5"}}`,
		`{"model":{"display_name":"Opus 5"},"effort":{"level":"turbo"}}`,
	} {
		line := runStatusline(t, input)
		if line != "Opus 5" {
			t.Errorf("statusline printed %q; want the bare model name", visible(line))
		}
	}
}

// The mark comes from the cache and only from it: the status line never goes to
// the network.
func TestStatuslineUpdateMark(t *testing.T) {
	testenv.Home(t)
	withVersion(t, "v1.0.0")
	if err := writeCache(updateCache{CheckedAt: time.Now().Unix(), Latest: "v9.9.9"}); err != nil {
		t.Fatalf("writeCache: %v", err)
	}

	line := statuslineOutput(t, `{"model":{"display_name":"Opus 5"}}`)
	parts := strings.Split(line, " · ")
	// Last, so that it never shifts the numbers the eye is used to.
	if len(parts) != 2 || !strings.Contains(parts[len(parts)-1], "↑ v9.9.9") {
		t.Errorf("statusline printed %q; want the update mark at the end", visible(line))
	}
}

// The snapshot is the only contract with the panel: the limits arrive on stdin
// and nowhere else, so the status line has to write them down as they came.
func TestStatuslineSavesTheSnapshot(t *testing.T) {
	testenv.Home(t)
	statuslineOutput(t, `{"rate_limits":{"five_hour":{"used_percentage":42,"resets_at":1893456000}}}`)

	var saved struct {
		RateLimits map[string]struct {
			UsedPercentage float64 `json:"used_percentage"`
			ResetsAt       float64 `json:"resets_at"`
		} `json:"rate_limits"`
		UpdatedAt string `json:"updated_at"`
	}
	data, err := os.ReadFile(snapshotFile(t))
	if err != nil {
		t.Fatalf("reading the snapshot: %v", err)
	}
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("the snapshot does not parse: %v", err)
	}

	five, ok := saved.RateLimits["five_hour"]
	if !ok || five.UsedPercentage != 42 || five.ResetsAt != 1893456000 {
		t.Errorf("the snapshot holds %+v; want the window as it arrived", saved.RateLimits)
	}
	if _, err := time.Parse(time.RFC3339, saved.UpdatedAt); err != nil {
		t.Errorf("updated_at = %q; want an RFC3339 time", saved.UpdatedAt)
	}
}

// Claude Code sometimes calls the status line with no limits at all: the
// previous numbers are more honest than empty ones.
func TestStatuslineKeepsTheSnapshotWhenNothingArrives(t *testing.T) {
	testenv.Home(t)
	statuslineOutput(t, `{"rate_limits":{"five_hour":{"used_percentage":42}}}`)
	before := readFile(t, snapshotFile(t))

	statuslineOutput(t, `{"model":{"display_name":"Opus 5"}}`)

	if after := readFile(t, snapshotFile(t)); after != before {
		t.Error("a call without limits overwrote the snapshot")
	}
}

func TestLabeledBar(t *testing.T) {
	const (
		green = "\x1b[1;48;5;35;38;5;16m"
		red   = "\x1b[1;48;5;167;38;5;231m"
		empty = "\x1b[0;48;5;237;38;5;250m"
		off   = "\x1b[0m"
	)

	cases := []struct {
		name       string
		percentage float64
		label      string
		color      int
		want       string
	}{
		// The label is centered, the leftover space going to the left: "12%"
		// drifts noticeably towards the left edge otherwise.
		{"nothing used", 0, "0%", colorGreen, green + "" + empty + "    0%    " + off},
		{"almost half", 42, "42%", colorGreen, green + "    " + empty + "42%   " + off},
		{"full", 100, "100%", colorGreen, green + "   100%   " + empty + "" + off},
		// On red the digits are white: black would drown in it.
		{"over the red line", 90, "90%", colorRed, red + "    90%  " + empty + " " + off},
		// A label longer than the bar is cut, not wrapped.
		{"a label that does not fit", 50, "0123456789+", colorGreen, green + "01234" + empty + "56789" + off},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := labeledBar(c.percentage, c.label, c.color)
			if got != c.want {
				t.Errorf("labeledBar(%v, %q) = %q;\n want %q", c.percentage, c.label, got, c.want)
			}
			// Whatever happens, the bar is exactly barWidth characters wide.
			if width := len([]rune(visible(got))); width != barWidth {
				t.Errorf("the bar is %d characters wide; want %d", width, barWidth)
			}
		})
	}
}

func TestUsageColor(t *testing.T) {
	cases := []struct {
		percentage float64
		want       int
	}{
		{0, colorGreen},
		{59.9, colorGreen},
		{60, colorOrange},
		{84.9, colorOrange},
		{85, colorRed},
		{100, colorRed},
	}

	for _, c := range cases {
		if got := usageColor(c.percentage); got != c.want {
			t.Errorf("usageColor(%v) = %d; want %d", c.percentage, got, c.want)
		}
	}
}

func TestPercentLabel(t *testing.T) {
	cases := []struct {
		percentage float64
		want       string
	}{
		{0, "0%"},
		{42.4, "42%"},
		{42.6, "43%"},
		{100, "100%"},
	}

	for _, c := range cases {
		if got := percentLabel(c.percentage); got != c.want {
			t.Errorf("percentLabel(%v) = %q; want %q", c.percentage, got, c.want)
		}
	}
}

// The hours are printed even when they are zero: without them the label shrinks
// from six characters to three as it crosses an hour, and the digits jump.
func TestCountdownKeepsItsWidth(t *testing.T) {
	widths := map[int]int{}
	for _, seconds := range []int{30, 60, 3599, 3600, 4 * 3600} {
		widths[len([]rune(countdown(seconds)))]++
	}
	if len(widths) != 1 {
		t.Errorf("the countdown changes width across the hour: %v", widths)
	}

	// The digits themselves are the same in every language.
	cases := map[int][2]int{
		0:            {0, 0},
		59:           {0, 0},
		60:           {0, 1},
		3599:         {0, 59},
		3600:         {1, 0},
		3661:         {1, 1},
		5*3600 - 1:   {4, 59},
		25*3600 + 61: {25, 1},
	}
	for seconds, want := range cases {
		got := countdown(seconds)
		hours, minutes := digits(got)
		if hours != want[0] || minutes != want[1] {
			t.Errorf("countdown(%d) = %q; want %dh %02dm", seconds, got, want[0], want[1])
		}
	}
}

func TestNormalizeEpoch(t *testing.T) {
	const seconds = 1893456000
	cases := []struct {
		value float64
		want  float64
	}{
		{seconds, seconds},
		// The same moment in milliseconds.
		{seconds * 1000, seconds},
		{0, 0},
	}

	for _, c := range cases {
		if got := normalizeEpoch(c.value); got != c.want {
			t.Errorf("normalizeEpoch(%v) = %v; want %v", c.value, got, c.want)
		}
	}
}

func TestSecondsLeft(t *testing.T) {
	t.Run("no reset time", func(t *testing.T) {
		if _, ok := secondsLeft(nil); ok {
			t.Error("a window without resets_at reported a countdown")
		}
	})

	t.Run("already reset", func(t *testing.T) {
		past := float64(time.Now().Add(-time.Second).Unix())
		if left, ok := secondsLeft(&past); ok {
			t.Errorf("a window that reset reported %d seconds left", left)
		}
	})

	t.Run("an hour to go", func(t *testing.T) {
		future := float64(time.Now().Add(time.Hour).Unix())
		left, ok := secondsLeft(&future)
		if !ok || left < 3595 || left > 3600 {
			t.Errorf("secondsLeft() = %d, %t; want about an hour", left, ok)
		}
	})

	t.Run("in milliseconds", func(t *testing.T) {
		future := float64(time.Now().Add(time.Hour).UnixMilli())
		left, ok := secondsLeft(&future)
		if !ok || left < 3595 || left > 3600 {
			t.Errorf("secondsLeft() = %d, %t; want about an hour", left, ok)
		}
	})
}

func TestParseWindow(t *testing.T) {
	t.Run("a full window", func(t *testing.T) {
		w := parseWindow(json.RawMessage(`{"used_percentage":42.5,"resets_at":1893456000}`))
		if w.UsedPercentage == nil || *w.UsedPercentage != 42.5 {
			t.Errorf("UsedPercentage = %v; want 42.5", w.UsedPercentage)
		}
		if w.ResetsAt == nil || *w.ResetsAt != 1893456000 {
			t.Errorf("ResetsAt = %v; want the epoch it was given", w.ResetsAt)
		}
	})

	// A missing field stays nil rather than becoming a zero: nought percent used
	// and no number at all are different things, and only one of them is drawn.
	for _, raw := range []string{"", "{}", "null", "not json", `{"used_percentage":null}`} {
		w := parseWindow(json.RawMessage(raw))
		if w.UsedPercentage != nil || w.ResetsAt != nil {
			t.Errorf("parseWindow(%q) = %+v; want an empty window", raw, w)
		}
	}
}

// runStatusline calls the status line in a home of its own, with input on
// stdin, and gives back the single line it printed.
func runStatusline(t *testing.T, input string) string {
	t.Helper()

	testenv.Home(t)
	return statuslineOutput(t, input)
}

// statuslineOutput does the same in the home the test has already set up — for
// the tests that call the status line twice, or that prepare a cache first.
func statuslineOutput(t *testing.T, input string) string {
	t.Helper()

	// No background check: it would spawn a process of its own, and the status
	// line is what is being watched here, not GitHub.
	t.Setenv("CLAUDESTATUS_NO_AUTO_UPDATE", "1")

	feedStdin(t, input)
	printed := testenv.CaptureStdout(t, statusline)

	return strings.TrimSuffix(printed, "\n")
}

func snapshotFile(t *testing.T) string {
	t.Helper()

	dir, err := paths.Dir()
	if err != nil {
		t.Fatalf("the application directory: %v", err)
	}
	return filepath.Join(dir, snapshotName)
}

// resetsIn — the epoch a window resets at, as Claude Code sends it.
func resetsIn(left time.Duration) int64 {
	return time.Now().Add(left).Unix()
}

// visible strips the color escapes: the tests care about what is written, not
// about how it is painted — except where they check the painting itself.
func visible(line string) string {
	var out strings.Builder
	for {
		start := strings.Index(line, "\x1b[")
		if start < 0 {
			out.WriteString(line)
			return out.String()
		}
		out.WriteString(line[:start])
		end := strings.IndexByte(line[start:], 'm')
		if end < 0 {
			return out.String()
		}
		line = line[start+end+1:]
	}
}

// digits reads the two numbers out of a countdown label, whatever words the
// translation puts around them.
func digits(label string) (int, int) {
	var numbers []int
	current, inNumber := 0, false
	for _, r := range label + " " {
		if r >= '0' && r <= '9' {
			current, inNumber = current*10+int(r-'0'), true
			continue
		}
		if inNumber {
			numbers = append(numbers, current)
			current, inNumber = 0, false
		}
	}
	if len(numbers) != 2 {
		return -1, -1
	}
	return numbers[0], numbers[1]
}

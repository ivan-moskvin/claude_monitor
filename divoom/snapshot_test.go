package divoom

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/ivan-moskvin/claude_monitor/internal/testenv"
)

func TestUsageWindowFractions(t *testing.T) {
	cases := []struct {
		name            string
		window          usageWindow
		fraction        float64
		elapsedFraction float64
	}{
		{"fresh window", usageWindow{present: true, used: 0, secondsLeft: fiveHourSeconds}, 0, 0},
		{"half used, half elapsed", usageWindow{present: true, used: 50, secondsLeft: fiveHourSeconds / 2}, 0.5, 0.5},
		{"full", usageWindow{present: true, used: 100, secondsLeft: 1}, 1, float64(fiveHourSeconds-1) / fiveHourSeconds},
		// A window that has reset shows a full reset bar and no usage: the new
		// window starts from zero and its numbers have not arrived yet.
		{"expired", usageWindow{present: true, expired: true}, 0, 1},
		// Without resets_at there is no countdown — the bar is drawn full rather
		// than empty, so it never looks like a window that just started.
		{"no countdown", usageWindow{present: true, used: 20}, 0.2, 1},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.window.fraction(); got != c.fraction {
				t.Errorf("fraction() = %v; want %v", got, c.fraction)
			}
			if got := c.window.elapsedFraction(); got != c.elapsedFraction {
				t.Errorf("elapsedFraction() = %v; want %v", got, c.elapsedFraction)
			}
		})
	}
}

func TestUsageWindowPercentLabel(t *testing.T) {
	cases := []struct {
		window usageWindow
		want   string
	}{
		// A window Claude Code never sent has no number to show.
		{usageWindow{}, "-"},
		{usageWindow{present: true}, "0%"},
		{usageWindow{present: true, used: 42.4}, "42%"},
		{usageWindow{present: true, used: 42.6}, "43%"},
		{usageWindow{present: true, used: 100}, "100%"},
	}

	for _, c := range cases {
		if got := c.window.percentLabel(); got != c.want {
			t.Errorf("percentLabel(%+v) = %q; want %q", c.window, got, c.want)
		}
	}
}

// The thresholds are the same as usageColor in the status line: the panel and
// the line show one thing and must not disagree about the color.
func TestUsageWindowTint(t *testing.T) {
	cases := []struct {
		window usageWindow
		want   uint8
	}{
		{usageWindow{}, idxGrey},
		{usageWindow{present: true, expired: true, used: 90}, idxGrey},
		{usageWindow{present: true, used: 0}, idxGreen},
		{usageWindow{present: true, used: 59.9}, idxGreen},
		{usageWindow{present: true, used: 60}, idxOrange},
		{usageWindow{present: true, used: 84.9}, idxOrange},
		{usageWindow{present: true, used: 85}, idxRed},
		{usageWindow{present: true, used: 100}, idxRed},
	}

	for _, c := range cases {
		if got := c.window.tint(); got != c.want {
			t.Errorf("tint(used=%v, expired=%t) = %d; want %d", c.window.used, c.window.expired, got, c.want)
		}
	}
}

// usageKey decides whether the frame is worth resending: the percentages change
// it, the ticking countdown does not.
func TestUsageKeyIgnoresTheCountdown(t *testing.T) {
	state := snapshot{windows: map[string]usageWindow{
		"five_hour": {present: true, used: 42, secondsLeft: 9000},
	}}
	ticked := withWindow(state, "five_hour", usageWindow{present: true, used: 42, secondsLeft: 8940})
	grown := withWindow(state, "five_hour", usageWindow{present: true, used: 43, secondsLeft: 9000})
	reset := withWindow(state, "five_hour", usageWindow{present: true, expired: true})

	if state.usageKey() != ticked.usageKey() {
		t.Error("a minute of the countdown changed the key — the frame would be resent for nothing")
	}
	if state.usageKey() == grown.usageKey() {
		t.Error("a grown percentage left the key alone — the panel would stay stale")
	}
	if state.usageKey() == reset.usageKey() {
		t.Error("a window that reset left the key alone")
	}
	if (snapshot{err: "no snapshot found"}).usageKey() == state.usageKey() {
		t.Error("an error state shares its key with a normal one")
	}
}

func TestReadSnapshotWithoutAFile(t *testing.T) {
	testenv.Home(t)

	if state := readSnapshot(); state.err == "" {
		t.Error("a missing snapshot went unreported")
	}
}

func TestReadSnapshotDamaged(t *testing.T) {
	testenv.Home(t)
	writeSnapshot(t, "{not json")

	if state := readSnapshot(); state.err == "" {
		t.Error("a damaged snapshot went unreported")
	}
}

func TestReadSnapshotWithoutLimits(t *testing.T) {
	testenv.Home(t)
	// A window without used_percentage carries nothing to draw.
	writeSnapshot(t, `{"rate_limits":{"five_hour":{"resets_at":1}},"updated_at":"2026-01-01T00:00:00Z"}`)

	if state := readSnapshot(); state.err == "" {
		t.Error("a snapshot with no usable window went unreported")
	}
}

func TestReadSnapshotWindows(t *testing.T) {
	testenv.Home(t)

	now := time.Now()
	writeSnapshot(t, fmt.Sprintf(
		`{"rate_limits":{
			"five_hour":{"used_percentage":42.5,"resets_at":%d},
			"seven_day":{"used_percentage":13},
			"seven_day_opus":{"used_percentage":90,"resets_at":%d}
		},"updated_at":%q}`,
		now.Add(time.Hour).Unix(),
		now.Add(-time.Minute).Unix(),
		now.UTC().Format(time.RFC3339)))

	state := readSnapshot()
	if state.err != "" {
		t.Fatalf("readSnapshot: %s", state.err)
	}

	five := state.window("five_hour")
	if !five.present || five.used != 42.5 || five.expired {
		t.Errorf("five_hour = %+v; want a live window at 42.5%%", five)
	}
	// Rounding down by a second or two is expected — the countdown is read at
	// the moment of the call.
	if five.secondsLeft < 3595 || five.secondsLeft > 3600 {
		t.Errorf("five_hour.secondsLeft = %d; want about an hour", five.secondsLeft)
	}

	// A window without resets_at is shown as it came, with no countdown.
	if week := state.window("seven_day"); !week.present || week.used != 13 || week.secondsLeft != 0 {
		t.Errorf("seven_day = %+v; want 13%% and no countdown", week)
	}

	// A window whose reset has passed starts from zero: the old percentage
	// describes a window that is over.
	opus := state.window("seven_day_opus")
	if !opus.expired || opus.used != 0 {
		t.Errorf("seven_day_opus = %+v; want an expired window at 0%%", opus)
	}

	// A window Claude Code never sent is simply absent.
	if missing := state.window("nothing_like_this"); missing.present {
		t.Errorf("an unknown window came back as present: %+v", missing)
	}
}

// resets_at arrives in seconds from Claude Code, but the same field has been
// seen in milliseconds; both have to mean the same moment.
func TestReadSnapshotAcceptsMilliseconds(t *testing.T) {
	testenv.Home(t)
	writeSnapshot(t, fmt.Sprintf(
		`{"rate_limits":{"five_hour":{"used_percentage":10,"resets_at":%d}},"updated_at":%q}`,
		time.Now().Add(time.Hour).UnixMilli(),
		time.Now().UTC().Format(time.RFC3339)))

	five := readSnapshot().window("five_hour")
	if five.expired || five.secondsLeft < 3595 || five.secondsLeft > 3600 {
		t.Errorf("five_hour = %+v; want about an hour left", five)
	}
}

func TestReadSnapshotStaleness(t *testing.T) {
	cases := []struct {
		name      string
		updatedAt time.Time
		stale     bool
	}{
		{"just written", time.Now(), false},
		{"a minute old", time.Now().Add(-time.Minute), false},
		// Claude Code only refreshes the snapshot during a session: an old one
		// describes the past, and the panel has to say so.
		{"older than the threshold", time.Now().Add(-2 * staleAfter), true},
		{"hours old", time.Now().Add(-3 * time.Hour), true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			testenv.Home(t)
			writeSnapshot(t, fmt.Sprintf(
				`{"rate_limits":{"five_hour":{"used_percentage":10}},"updated_at":%q}`,
				c.updatedAt.UTC().Format(time.RFC3339)))

			state := readSnapshot()
			if state.stale != c.stale {
				t.Errorf("stale = %t; want %t (age %s)", state.stale, c.stale, state.age)
			}
		})
	}
}

func writeSnapshot(t *testing.T, content string) {
	t.Helper()

	path, err := snapshotPath()
	if err != nil {
		t.Fatalf("snapshotPath: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing the snapshot: %v", err)
	}
}

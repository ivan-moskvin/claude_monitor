package divoom

import (
	"bytes"
	"image/gif"
	"maps"
	"testing"
	"time"
)

func TestCountdownLabel(t *testing.T) {
	cases := []struct {
		seconds int
		want    string
	}{
		// Nothing left to count down to — the bar says so instead of showing 0:00.
		{0, "-"},
		{-30, "-"},
		{59, "0:00"},
		{60, "0:01"},
		{3599, "0:59"},
		{3600, "1:00"},
		{3661, "1:01"},
		{5 * 3600, "5:00"},
	}

	for _, c := range cases {
		if got := countdownLabel(c.seconds); got != c.want {
			t.Errorf("countdownLabel(%d) = %q; want %q", c.seconds, got, c.want)
		}
	}
}

func TestAgeLabel(t *testing.T) {
	cases := []struct {
		age  time.Duration
		want string
	}{
		{30 * time.Second, "0M"},
		{90 * time.Second, "1M"},
		{59 * time.Minute, "59M"},
		{time.Hour, "1H"},
		{90 * time.Minute, "1H"},
		{25 * time.Hour, "25H"},
	}

	for _, c := range cases {
		// The label is drawn, not spoken: the format carries no words, so the
		// language does not change the digits.
		if got := ageLabel(c.age); got != c.want {
			t.Errorf("ageLabel(%s) = %q; want %q", c.age, got, c.want)
		}
	}
}

func TestResetBarFollowsTheWindow(t *testing.T) {
	live := usageWindow{present: true, secondsLeft: 3600}
	if got := resetLabel(live); got != "1:00" {
		t.Errorf("resetLabel(live) = %q; want the countdown", got)
	}
	if got := resetTint(live); got != idxCyan {
		t.Errorf("resetTint(live) = %d; want cyan %d", got, idxCyan)
	}

	// A window that has reset is dimmed: there is no time left to count.
	expired := usageWindow{present: true, expired: true}
	if got := resetTint(expired); got != idxGrey {
		t.Errorf("resetTint(expired) = %d; want grey %d", got, idxGrey)
	}
	if resetLabel(expired) == countdownLabel(0) {
		t.Error("an expired window shows a countdown instead of the reset label")
	}
}

func TestRenderProducesA128PixelGIF(t *testing.T) {
	data, hash, err := render(sampleSnapshot())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if hash == "" {
		t.Error("render returned an empty hash — the device would never re-fetch the frame")
	}

	img, err := gif.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("the frame does not decode as a GIF: %v", err)
	}
	if bounds := img.Bounds(); bounds.Dx() != panelSize || bounds.Dy() != panelSize {
		t.Errorf("the frame is %dx%d; the device takes %d", bounds.Dx(), bounds.Dy(), panelSize)
	}
}

// The hash names the file the device downloads: the same numbers must give the
// same name, or the panel is re-fetched on every tick.
func TestRenderIsDeterministic(t *testing.T) {
	_, first, err := render(sampleSnapshot())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	_, second, err := render(sampleSnapshot())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if first != second {
		t.Errorf("the same snapshot rendered to %s and then to %s", first, second)
	}
}

// And a changed state has to give a new name, or the device keeps showing the
// frame it already has.
func TestRenderReactsToTheState(t *testing.T) {
	base := sampleSnapshot()
	_, unchanged, _ := render(base)

	cases := map[string]snapshot{
		"another percentage": withWindow(base, "five_hour", usageWindow{present: true, used: 61, secondsLeft: 9000}),
		"the window reset":   withWindow(base, "five_hour", usageWindow{present: true, expired: true}),
		"the weekly window":  withWindow(base, "seven_day", usageWindow{present: true, used: 90}),
		"a stale snapshot":   {windows: base.windows, stale: true, age: 3 * time.Hour},
		"no data at all":     {err: "no snapshot found"},
	}

	for name, state := range cases {
		t.Run(name, func(t *testing.T) {
			_, changed, err := render(state)
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			if changed == unchanged {
				t.Error("the frame did not change with the state")
			}
		})
	}
}

func sampleSnapshot() snapshot {
	return snapshot{windows: map[string]usageWindow{
		"five_hour": {present: true, used: 42, secondsLeft: 9000},
		"seven_day": {present: true, used: 13},
	}}
}

func withWindow(state snapshot, id string, w usageWindow) snapshot {
	windows := make(map[string]usageWindow, len(state.windows))
	maps.Copy(windows, state.windows)
	windows[id] = w
	return snapshot{windows: windows, stale: state.stale, age: state.age, err: state.err}
}

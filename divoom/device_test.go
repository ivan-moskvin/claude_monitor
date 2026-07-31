package divoom

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The device answers HTTP 200 to everything, so the real result is in
// error_code — sometimes a number, sometimes the text of the error.
func TestDeviceCall(t *testing.T) {
	cases := []struct {
		name    string
		answer  string
		wantErr bool
	}{
		{name: "accepted", answer: `{"error_code":0}`},
		{name: "accepted as a string", answer: `{"error_code":"0"}`},
		{name: "no code at all", answer: `{}`},
		{name: "rejected", answer: `{"error_code":1}`, wantErr: true},
		{name: "an unknown command", answer: `{"error_code":"Request data illegal json"}`, wantErr: true},
		{name: "not JSON", answer: `<html>the router says hello</html>`, wantErr: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			target, _ := fakeDevice(t, c.answer)

			err := target.call(map[string]any{"Command": "Channel/GetAllConf"})
			if (err != nil) != c.wantErr {
				t.Errorf("call() error = %v; wantErr %t", err, c.wantErr)
			}
		})
	}
}

// The address of the device goes into a URL, where an IPv6 literal has to be
// bracketed — but it may already carry a port, and then it is left alone.
func TestPostURL(t *testing.T) {
	cases := map[string]string{
		"192.168.0.2":        "http://192.168.0.2/post",
		"127.0.0.1:52345":    "http://127.0.0.1:52345/post",
		"fd00::2":            "http://[fd00::2]/post",
		"[fd00::2]:52345":    "http://[fd00::2]:52345/post",
		"::ffff:192.168.0.2": "http://[::ffff:192.168.0.2]/post",
	}

	for address, want := range cases {
		if got := postURL(address); got != want {
			t.Errorf("postURL(%q) = %q; want %q", address, got, want)
		}
	}
}

func TestDeviceCallUnreachable(t *testing.T) {
	// Nothing listens on this address: the device is switched off.
	target := device{ip: "127.0.0.1:1", lcd: 4}

	if err := target.call(map[string]any{"Command": "Channel/GetAllConf"}); err == nil {
		t.Error("a switched-off device answered without complaint")
	}
}

// The frame is not pushed to the device: it is handed a link and comes for the
// picture itself. Which screen gets it is an array with a single one in it.
func TestShowGif(t *testing.T) {
	target, requests := fakeDevice(t, `{"error_code":0}`)
	target.lcd = 2

	if err := target.showGif("http://192.168.0.2:8477/panel-abc123.gif"); err != nil {
		t.Fatalf("showGif: %v", err)
	}

	payload := (<-requests)
	if payload["Command"] != "Device/PlayGif" {
		t.Errorf("Command = %v; want Device/PlayGif", payload["Command"])
	}
	want := []any{0.0, 0.0, 1.0, 0.0, 0.0}
	if got, _ := payload["LcdArray"].([]any); !equalArrays(got, want) {
		t.Errorf("LcdArray = %v; want %v", payload["LcdArray"], want)
	}
	if files, _ := payload["FileName"].([]any); len(files) != 1 || files[0] != "http://192.168.0.2:8477/panel-abc123.gif" {
		t.Errorf("FileName = %v; want the link to the frame", payload["FileName"])
	}
}

// Without the previous clock face the screen is left with a dead picture, so a
// restore we cannot do has to be said out loud rather than sent as a zero.
func TestRestoreScreenWithoutAClockFace(t *testing.T) {
	target, requests := fakeDevice(t, `{"error_code":0}`)

	if err := target.restoreScreen(0, 1); err == nil {
		t.Error("restoreScreen sent a command with no clock face to restore")
	}
	select {
	case payload := <-requests:
		t.Errorf("the device was bothered anyway: %v", payload)
	default:
	}
}

func TestRestoreScreen(t *testing.T) {
	target, requests := fakeDevice(t, `{"error_code":0}`)
	target.lcd = 3

	if err := target.restoreScreen(61, 1); err != nil {
		t.Fatalf("restoreScreen: %v", err)
	}

	payload := <-requests
	if payload["Command"] != "Channel/SetClockSelectId" {
		t.Errorf("Command = %v; want Channel/SetClockSelectId", payload["Command"])
	}
	if payload["ClockId"] != 61.0 || payload["LcdIndex"] != 3.0 || payload["LcdIndependence"] != 1.0 {
		t.Errorf("the screen was restored with %v", payload)
	}
}

func TestLayout(t *testing.T) {
	var asked string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.RequestURI()
		_, _ = w.Write([]byte(`{
			"LcdIndependence": 1,
			"LcdIndependenceList": [{"LcdList": [
				{"LcdClockId": 61}, {"LcdClockId": 62}, {"LcdClockId": 63},
				{"LcdClockId": 64}, {"LcdClockId": 65}
			]}]
		}`))
	}))
	defer server.Close()
	withLcdInfoAPI(t, server.URL)

	clocks, independence, err := layout(300000123)
	if err != nil {
		t.Fatalf("layout: %v", err)
	}
	if len(clocks) != 5 || clocks[4] != 65 {
		t.Errorf("layout() = %v; want the five clock faces of the device", clocks)
	}
	if independence != 1 {
		t.Errorf("independence = %d; want 1", independence)
	}
	if !strings.Contains(asked, "DeviceId=300000123") {
		t.Errorf("the layout was asked for with %q; want our device id in it", asked)
	}
}

func TestLayoutWithoutAnAnswer(t *testing.T) {
	cases := map[string]string{
		"an empty layout": `{"LcdIndependence":1,"LcdIndependenceList":[]}`,
		"not JSON":        `<html>`,
	}

	for name, answer := range cases {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(answer))
			}))
			defer server.Close()
			withLcdInfoAPI(t, server.URL)

			if _, _, err := layout(300000123); err == nil {
				t.Error("an answer with no layout in it passed for one")
			}
		})
	}
}

// fakeDevice stands in for a Times Gate: it answers every command with the
// given JSON and hands the payloads it was sent back to the test.
func fakeDevice(t *testing.T, answer string) (device, chan map[string]any) {
	t.Helper()

	requests := make(chan map[string]any, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		if json.Unmarshal(body, &payload) == nil {
			requests <- payload
		}
		_, _ = w.Write([]byte(answer))
	}))
	t.Cleanup(server.Close)

	return device{ip: strings.TrimPrefix(server.URL, "http://"), lcd: 4}, requests
}

func withLcdInfoAPI(t *testing.T, url string) {
	t.Helper()

	saved := lcdInfoAPI
	lcdInfoAPI = url
	t.Cleanup(func() { lcdInfoAPI = saved })
}

func equalArrays(got, want []any) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

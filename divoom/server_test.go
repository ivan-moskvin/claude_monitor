package divoom

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestPublishBuildsALinkForTheDevice(t *testing.T) {
	server := newAssets(8477, "192.168.0.5")
	url := server.publish("192.168.0.2", "abc123", []byte("a frame"))

	if url != "http://192.168.0.2:8477/panel-abc123.gif" {
		t.Errorf("publish() = %q", url)
	}
	// The name carries the hash: the device only re-downloads a frame that
	// really changed.
	if !strings.Contains(url, "abc123") {
		t.Errorf("the link does not name the frame: %q", url)
	}
}

// Only the last few frames are kept: the device comes for the file with a delay
// and may re-read it after a reboot, so the link must not go stale at once —
// but the frames must not pile up either.
func TestPublishKeepsTheLastFrames(t *testing.T) {
	server := newAssets(8477, "")

	var links []string
	for i := 0; i < keepFrames+2; i++ {
		links = append(links, server.publish("192.168.0.2", "hash"+strconv.Itoa(i), []byte("frame")))
	}

	server.mu.Lock()
	kept := len(server.frames)
	server.mu.Unlock()
	if kept != keepFrames {
		t.Errorf("%d frames kept; want %d", kept, keepFrames)
	}

	// The newest links still work, the oldest are gone.
	for _, link := range links[len(links)-keepFrames:] {
		if status, _ := fetch(t, server, link, "192.168.0.5"); status != http.StatusOK {
			t.Errorf("a recent frame answered %d", status)
		}
	}
	if status, _ := fetch(t, server, links[0], "192.168.0.5"); status != http.StatusNotFound {
		t.Errorf("the oldest frame is still served, answered %d", status)
	}
}

// Publishing the same frame twice must not push the other frames out: the panel
// stands still for minutes at a time.
func TestPublishIgnoresARepeat(t *testing.T) {
	server := newAssets(8477, "")
	first := server.publish("192.168.0.2", "abc123", []byte("a frame"))
	second := server.publish("192.168.0.2", "abc123", []byte("a frame"))

	if first != second {
		t.Errorf("the same frame got two links: %q and %q", first, second)
	}
	server.mu.Lock()
	kept := len(server.order)
	server.mu.Unlock()
	if kept != 1 {
		t.Errorf("the frame was remembered %d times", kept)
	}
}

// The panel shows how much of the limit is used up: in a café subnet there are
// as many neighbours as devices, and none of them is getting a look.
func TestServeAnswersTheDeviceOnly(t *testing.T) {
	server := newAssets(8477, "192.168.0.5")
	link := server.publish("192.168.0.2", "abc123", []byte("a frame"))

	status, body := fetch(t, server, link, "192.168.0.5")
	if status != http.StatusOK || body != "a frame" {
		t.Errorf("the device got %d %q; want the frame", status, body)
	}

	if status, _ := fetch(t, server, link, "192.168.0.77"); status != http.StatusNotFound {
		t.Errorf("a neighbour got %d; want a closed door", status)
	}

	// After the device turns up at another address, that is the one served.
	server.allow("192.168.0.77")
	if status, _ := fetch(t, server, link, "192.168.0.77"); status != http.StatusOK {
		t.Errorf("the device at its new address got %d", status)
	}
	if status, _ := fetch(t, server, link, "192.168.0.5"); status != http.StatusNotFound {
		t.Errorf("the old address is still served: %d", status)
	}
}

func TestServeUnknownFrame(t *testing.T) {
	server := newAssets(8477, "")

	if status, _ := fetch(t, server, "http://192.168.0.2:8477/panel-nothing.gif", "192.168.0.5"); status != http.StatusNotFound {
		t.Errorf("a frame that was never published answered %d", status)
	}
}

// The command reaches the device instantly while the picture travels in a
// separate request: only that request proves the frame was taken.
func TestAwaitFetch(t *testing.T) {
	server := newAssets(8477, "192.168.0.5")
	link := server.publish("192.168.0.2", "abc123", []byte("a frame"))

	go func() {
		time.Sleep(10 * time.Millisecond)
		fetch(t, server, link, "192.168.0.5")
	}()
	if !server.awaitFetch(link, 2*time.Second) {
		t.Error("the frame was taken but awaitFetch did not notice")
	}
}

func TestAwaitFetchGivesUp(t *testing.T) {
	server := newAssets(8477, "192.168.0.5")
	link := server.publish("192.168.0.2", "abc123", []byte("a frame"))

	if server.awaitFetch(link, 50*time.Millisecond) {
		t.Error("awaitFetch reported a frame nobody came for")
	}
}

// A frame somebody else took does not count as ours: the bridge would exit
// believing the device has the picture.
func TestAwaitFetchIgnoresAnotherFrame(t *testing.T) {
	server := newAssets(8477, "192.168.0.5")
	mine := server.publish("192.168.0.2", "mine", []byte("a frame"))
	theirs := server.publish("192.168.0.2", "theirs", []byte("another frame"))

	fetch(t, server, theirs, "192.168.0.5")
	if server.awaitFetch(mine, 100*time.Millisecond) {
		t.Error("a different frame was taken for ours")
	}
}

// The port is taken before the first command goes out: a server that failed
// silently turns into endless loading on the screen with no hint of the reason.
func TestListenFallsBackToAFreePort(t *testing.T) {
	occupied, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("occupying a port: %v", err)
	}
	defer occupied.Close()
	taken := occupied.Addr().(*net.TCPAddr).Port

	server := newAssets(taken, "")
	// The message about the port goes to stdout and is not what is being
	// checked here.
	if err := server.listen(); err != nil {
		t.Fatalf("listen: %v", err)
	}
	if server.port == taken {
		t.Errorf("listen stayed on the port somebody else holds: %d", taken)
	}
	if server.port == 0 {
		t.Error("listen reported no port at all")
	}
}

// fetch asks the server for a frame as the device at ip would.
func fetch(t *testing.T, server *assets, link, ip string) (int, string) {
	t.Helper()

	path := link[strings.LastIndex(link, "/"):]
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.RemoteAddr = net.JoinHostPort(ip, "51234")

	recorder := httptest.NewRecorder()
	server.serve(recorder, request)

	body, err := io.ReadAll(recorder.Result().Body)
	if err != nil {
		t.Fatalf("reading the answer: %v", err)
	}
	return recorder.Code, string(body)
}

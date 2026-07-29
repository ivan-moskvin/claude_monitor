package divoom

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ivan-moskvin/claude_monitor/i18n"
)

// How many past frames are kept available. The device comes for the file with a
// delay and may re-read it after a reboot of its own — if the link has gone
// stale by then, the screen is left empty.
const keepFrames = 3

// assets — the HTTP server the device takes the frames from. The port stays the
// same: after the bridge restarts, the screen still holds a link from the
// previous run.
type assets struct {
	port int
	// Who is allowed to take a frame: the panel shows limit usage, and in a
	// café subnet there are as many neighbours as devices.
	allowIP string
	// The paths the device has already come for. The command is delivered
	// instantly while the picture travels in a separate request — and only that
	// request proves the frame was really taken.
	fetched chan string

	mu     sync.Mutex
	frames map[string][]byte
	order  []string
}

func newAssets(port int, allowIP string) *assets {
	return &assets{port: port, allowIP: allowIP, frames: make(map[string][]byte), fetched: make(chan string, 8)}
}

// awaitFetch waits until the device has taken the frame behind this link.
func (a *assets) awaitFetch(url string, timeout time.Duration) bool {
	deadline := time.After(timeout)
	for {
		select {
		case path := <-a.fetched:
			if strings.HasSuffix(url, path) {
				return true
			}
		case <-deadline:
			return false
		}
	}
}

// listen takes the port before the first command goes to the device: a server
// that fails silently turns into endless loading on the screen with no hint of
// the reason.
func (a *assets) listen() error {
	socket, err := net.Listen("tcp", ":"+strconv.Itoa(a.port))
	if err != nil {
		// The port is taken by somebody else — grab any free one and remember
		// it, so that the link on the screen keeps working after the bridge
		// restarts.
		socket, err = net.Listen("tcp", ":0")
		if err != nil {
			return fmt.Errorf(i18n.T("could not open a port for the frames: %w"), err)
		}
		a.port = socket.Addr().(*net.TCPAddr).Port
		fmt.Printf(i18n.T("The port is taken, serving frames on %d\n"), a.port)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", a.serve)

	// Timeouts, so that a stuck connection does not hold the bridge: we serve
	// one client with one small file.
	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	go func() {
		_ = server.Serve(socket)
	}()
	return nil
}

func (a *assets) serve(w http.ResponseWriter, r *http.Request) {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err != nil || (a.allowIP != "" && host != a.allowIP) {
		http.NotFound(w, r)
		return
	}

	a.mu.Lock()
	data, ok := a.frames[r.URL.Path]
	a.mu.Unlock()

	if !ok {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "image/gif")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)

	select {
	case a.fetched <- r.URL.Path:
	default:
	}
}

// publish stores a frame and returns the link to it. The hash in the name is
// what makes the device re-download the picture: without a change of address it
// shows the previous one.
func (a *assets) publish(hostIP, hash string, data []byte) string {
	path := "/panel-" + hash + ".gif"

	a.mu.Lock()
	if _, exists := a.frames[path]; !exists {
		a.frames[path] = data
		a.order = append(a.order, path)
		for len(a.order) > keepFrames {
			delete(a.frames, a.order[0])
			a.order = a.order[1:]
		}
	}
	a.mu.Unlock()

	return fmt.Sprintf("http://%s:%d%s", hostIP, a.port, path)
}

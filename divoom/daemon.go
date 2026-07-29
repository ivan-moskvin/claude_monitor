package divoom

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/ivan-moskvin/claude_monitor/i18n"
	"github.com/ivan-moskvin/claude_monitor/paths"
)

// The device only refreshes the panel while the bridge is running. Nobody is
// going to start it by hand, so the status line starts it itself — but only
// when a Times Gate really is on the network. No device, no process.
const (
	lockName = "divoom.pid"
	// Checked on every call of the status line: it has to go unnoticed.
	probeTimeout = 400 * time.Millisecond
	// The device was switched off — the bridge does not hang around forever: it
	// exits, and the status line starts it again once the Times Gate is back.
	giveUpAfter = 10 * time.Minute
)

func lockPath() (string, error) {
	return paths.File(lockName)
}

// EnsureRunning starts the bridge in the background if the device is reachable
// and the bridge is not up yet. It is called from the status line and has to
// keep quiet: no trouble here is worth spoiling the line.
func EnsureRunning() {
	cfg, err := loadConfig()
	if err != nil || cfg.IP == "" {
		// The panel is not turned on — not an error, most people have no Times
		// Gate.
		return
	}
	if running() {
		return
	}
	if cfg.IP == "" || !reachable(cfg.IP) {
		return
	}

	exe, err := os.Executable()
	if err != nil {
		return
	}
	cmd := exec.Command(exe, "divoom")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	cmd.SysProcAttr = detachAttrs()
	_ = cmd.Start()
	if cmd.Process != nil {
		_ = cmd.Process.Release()
	}
}

func running() bool {
	path, err := lockPath()
	if err != nil {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return false
	}
	return processAlive(pid)
}

func takeLock() error {
	path, err := lockPath()
	if err != nil {
		return err
	}
	if running() {
		return errors.New(i18n.T("the bridge is already running"))
	}
	return os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644)
}

// ownsLock tells whether the pid file still points at us. A bridge that was
// replaced by a newer one, or whose file was removed by `divoom off`, has to
// go: otherwise it keeps drawing on a screen nobody expects it to touch, and
// stopping it is a hunt for a process nobody remembers starting.
func ownsLock() bool {
	path, err := lockPath()
	if err != nil {
		return true
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	return err == nil && pid == os.Getpid()
}

func dropLock() {
	if path, err := lockPath(); err == nil {
		_ = os.Remove(path)
	}
}

// Stop stops the bridge and cleans up after it. Called from
// `claudestatus uninstall`: without it the bridge outlives the deleted binary,
// while the screen of the device keeps the panel and goes into endless loading
// once the bridge does die.
func Stop() {
	path, err := lockPath()
	if err != nil {
		return
	}

	if data, err := os.ReadFile(path); err == nil {
		if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && pid > 0 {
			// Where there are signals the bridge restores the screen itself;
			// where there are none — and where it did not manage to — we do it
			// for it.
			_ = terminate(pid)
			for i := 0; i < 30 && processAlive(pid); i++ {
				time.Sleep(100 * time.Millisecond)
			}
			if processAlive(pid) {
				_ = forceKill(pid)
			}
			if !gracefulStop || processAlive(pid) {
				restore()
			}
			fmt.Println(i18n.T("The Divoom bridge is stopped"))
		}
	}
	_ = os.Remove(path)
}

// restore gives the screen back its previous clock face from the values saved
// in the config. It keeps quiet: during an uninstall the device may be switched
// off, and that is no reason to fail.
func restore() {
	cfg, err := loadConfig()
	if err != nil || cfg.IP == "" || cfg.PrevClockID == 0 {
		return
	}
	target := device{ip: cfg.IP, lcd: cfg.LcdIndex}
	_ = target.restoreScreen(cfg.PrevClockID, cfg.PrevIndependence)
}

// reachable — does the device answer on its port. We send no real command: the
// status line must not wait for the firmware to reply.
func reachable(ip string) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, "80"), probeTimeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

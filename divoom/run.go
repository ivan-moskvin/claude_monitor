// Package divoom — the `claudestatus divoom` subcommand: a panel with the
// Claude limits on the screen of a Divoom Times Gate.
//
// It reads the same usage snapshot the status line writes, draws a 128×128
// panel and hands it to the device. The device downloads the frame itself: the
// bridge brings up a local HTTP server and sends the link with a Device/PlayGif
// command — pushing pixels (Draw/SendHttpGif) is accepted by the Times Gate
// firmware but draws nothing.
package divoom

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ivan-moskvin/claude_monitor/i18n"
)

const (
	// The snapshot is rewritten on every call of the status line, that is
	// often: poll briskly, reading a file costs nothing.
	pollInterval = 5 * time.Second
	// The device blinks its loading indicator on every frame, so a shift of the
	// countdown waits this long. Changed percentages do not honour the
	// threshold: they are why the panel hangs on the wall at all.
	minSendInterval = 30 * time.Second
	// Resend the last frame from time to time: the device may have rebooted or
	// lost the picture while the snapshot stayed the same for weeks.
	resendAfter = 15 * time.Minute
	// How long we wait for the device to come and take the frame.
	fetchTimeout = 20 * time.Second
)

const usage = `claudestatus divoom — limits panel on the screen of a Divoom Times Gate.

Usage:
  claudestatus divoom on           find the Times Gate on the network and turn the panel on
  claudestatus divoom off          turn the panel off and give the screen its clock face back
  claudestatus divoom              keep the panel updated (works while running)
  claudestatus divoom once         send the panel once and exit
  claudestatus divoom screen [N]   show or take over screen 1–5
  claudestatus divoom preview FILE save a frame to a file without touching the device

The settings are divoom.json in the application directory, created by on.
`

// Run — the entry point of the subcommand. Errors are returned upwards: it is
// the CLI, not the package, that prints them and picks the exit code.
func Run(args []string) error {
	if len(args) == 0 {
		return run(false)
	}

	switch args[0] {
	case "on":
		return on()
	case "off":
		return off()
	case "once":
		return run(true)
	case "screen":
		return screen(args[1:])
	case "preview":
		if len(args) < 2 {
			return errors.New(i18n.T("name a file: claudestatus divoom preview panel.gif"))
		}
		data, _, err := render(readSnapshot())
		if err != nil {
			return err
		}
		return os.WriteFile(args[1], data, 0o644)
	case "help", "--help", "-h":
		fmt.Print(i18n.T(usage))
		return nil
	default:
		return fmt.Errorf(i18n.T("unknown command: %s\n\n%s"), args[0], i18n.T(usage))
	}
}

func run(once bool) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	if cfg.IP == "" {
		ip, name, deviceID, err := discover()
		if err != nil {
			return err
		}
		fmt.Printf(i18n.T("Found device %s: %s\n"), name, ip)
		cfg.IP, cfg.DeviceID = ip, deviceID
		if err := cfg.save(); err != nil {
			return err
		}
	}

	// The device id may have missed the config: with a hand-written IP the
	// discovery never ran, and without the id there is no way to ask for the
	// screen layout.
	if cfg.DeviceID == 0 {
		if _, _, deviceID, err := discover(); err == nil {
			cfg.DeviceID = deviceID
			_ = cfg.save()
		}
	}

	// Remember the clock face that was there before we take the screen: come
	// uninstall time there would be nowhere else to get it from.
	if cfg.PrevClockID == 0 && cfg.DeviceID != 0 {
		if clocks, independence, err := layout(cfg.DeviceID); err == nil && cfg.LcdIndex < len(clocks) {
			cfg.PrevClockID, cfg.PrevIndependence = clocks[cfg.LcdIndex], independence
			_ = cfg.save()
		}
	}

	host, err := localIP(cfg.IP)
	if err != nil {
		return fmt.Errorf(i18n.T("could not determine our own address on the device network: %w"), err)
	}

	server := newAssets(cfg.Port, cfg.IP)
	if err := server.listen(); err != nil {
		return err
	}
	if server.port != cfg.Port {
		cfg.Port = server.port
		_ = cfg.save()
	}

	target := device{ip: cfg.IP, lcd: cfg.LcdIndex}

	var lastHash, lastUsage string
	var lastSent time.Time

	send := func(wait bool) error {
		state := readSnapshot()
		usage := state.usageKey()

		data, hash, err := render(state)
		if err != nil {
			return err
		}

		switch {
		case hash == lastHash:
			// Same frame — resend it rarely, so that the device does not lose
			// the picture after a reboot of its own.
			if time.Since(lastSent) < resendAfter {
				return nil
			}
		case usage == lastUsage && !lastSent.IsZero() && time.Since(lastSent) < minSendInterval:
			// Only the countdown moved — not worth bothering the device.
			return nil
		}

		url := server.publish(host, hash, data)
		if err := target.showGif(url); err != nil {
			return err
		}
		lastHash, lastUsage, lastSent = hash, usage, time.Now()

		// The command is delivered instantly, but the device comes for the
		// picture in a separate request — a one-shot send must not exit
		// earlier, or the server closes before the frame is taken.
		if wait && !server.awaitFetch(url, fetchTimeout) {
			return fmt.Errorf(i18n.T("the device did not take the frame within %s"), fetchTimeout)
		}
		return nil
	}

	if err := send(once); err != nil {
		return err
	}
	if once {
		return nil
	}

	// A second bridge to the same device would interleave its frames with the
	// first one.
	if err := takeLock(); err != nil {
		return err
	}
	defer dropLock()

	// On a signal we give the screen its previous clock face back: otherwise it
	// keeps the last frame, with nobody left to come for it.
	stopping := make(chan os.Signal, 1)
	signal.Notify(stopping, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stopping
		_ = target.restoreScreen(cfg.PrevClockID, cfg.PrevIndependence)
		dropLock()
		os.Exit(0)
	}()

	fmt.Printf(i18n.T("Panel on screen %d of device %s, updated every %s\n"),
		cfg.LcdIndex, cfg.IP, pollInterval)

	// Errors inside the loop do not bring the bridge down: the device may have
	// been switched off for the night. But hanging around a switched-off device
	// forever is pointless too — after giveUpAfter the bridge leaves, and the
	// status line starts it again once the Times Gate is back on the network.
	lastSeen := time.Now()
	for range time.Tick(pollInterval) {
		if err := send(false); err != nil {
			fmt.Fprintln(os.Stderr, err)
			if time.Since(lastSeen) > giveUpAfter {
				return fmt.Errorf(i18n.T("the device has been unreachable for more than %s, leaving"), giveUpAfter)
			}
			continue
		}
		lastSeen = time.Now()
	}
	return nil
}

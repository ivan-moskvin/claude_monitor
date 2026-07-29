package divoom

import (
	"fmt"
	"os"
	"time"

	"github.com/ivan-moskvin/claude_monitor/i18n"
)

// on finds the Times Gate on the network and turns the panel on.
func on() error {
	ip, name, deviceID, err := discover()
	if err != nil {
		return err
	}

	cfg, _ := loadConfig()
	cfg.IP, cfg.DeviceID = ip, deviceID
	if cfg.Port == 0 {
		cfg.Port = defaultPort
	}
	if err := cfg.save(); err != nil {
		return err
	}

	fmt.Printf(i18n.T("Found device %s: %s\n"), name, ip)

	EnsureRunning()
	// The bridge writes its pid file a moment after the fork, so a check right
	// away would report failure on a bridge that is in fact starting.
	for i := 0; i < 30 && !running(); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	if running() {
		fmt.Printf(i18n.T("The panel is on, screen %d\n"), cfg.LcdIndex+1)
	}
	return nil
}

func off() error {
	if running() {
		Stop()
	} else {
		restore()
	}

	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	fmt.Println(i18n.T("The panel is off, the screen got its clock face back"))
	return nil
}

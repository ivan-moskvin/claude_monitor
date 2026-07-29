package divoom

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/ivan-moskvin/claude_monitor/i18n"
	"github.com/ivan-moskvin/claude_monitor/paths"
)

const configName = "divoom.json"

// The default port of the local server with the frames.
const defaultPort = 8477

type config struct {
	// The IP of the device. Empty — look for it on the network again.
	IP string `json:"ip"`
	// Screen 0–4, the one we hand the panel to. The others are left alone.
	LcdIndex int `json:"lcd_index"`
	// The port of the local server with the frames; if taken, a free one is used.
	Port int `json:"port"`
	// The id of the device in the Divoom cloud — the screen layout is looked up by it.
	DeviceID int `json:"device_id,omitempty"`
	// What was on our screen before us: the clock face and the set of screens it
	// belongs to. Remembered on the first run and given back on uninstall —
	// otherwise the screen is left with a dead picture once the bridge is gone.
	PrevClockID      int `json:"prev_clock_id,omitempty"`
	PrevIndependence int `json:"prev_independence,omitempty"`
}

func configPath() (string, error) {
	return paths.File(configName)
}

func loadConfig() (config, error) {
	cfg := config{LcdIndex: 4, Port: defaultPort}

	path, err := configPath()
	if err != nil {
		return cfg, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, errors.New(i18n.T("the panel is not turned on — claudestatus divoom on"))
		}
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf(i18n.T("%s does not parse: %w"), path, err)
	}
	if cfg.Port == 0 {
		cfg.Port = defaultPort
	}
	return cfg, nil
}

func (c config) save() error {
	path, err := configPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

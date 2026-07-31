package divoom

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/ivan-moskvin/claudestatus/i18n"
	"github.com/ivan-moskvin/claudestatus/paths"
)

const configName = "divoom.json"

// The default port of the local server with the frames.
const defaultPort = 8477

type config struct {
	// Whether the panel is turned on. `off` clears the flag but keeps the file:
	// the chosen device and the chosen screen have to survive it, or every `on`
	// would start over from the defaults. A config written before the flag
	// existed carries no value and counts as on.
	On *bool `json:"on,omitempty"`
	// The last known IP of the device. DHCP moves it, so it is a hint for the
	// search, never the answer.
	IP string `json:"ip"`
	// Screen 0–4, the one we hand the panel to. The others are left alone.
	LcdIndex int `json:"lcd_index"`
	// The port of the local server with the frames; if taken, a free one is used.
	Port int `json:"port"`
	// The id of the device in the Divoom cloud — the screen layout is looked up
	// by it, and it tells our device from the neighbouring ones after a move.
	DeviceID int `json:"device_id,omitempty"`
	// The MAC and the name of the chosen device: the MAC identifies it when the
	// directory is silent, the name is what the human sees.
	MAC  string `json:"mac,omitempty"`
	Name string `json:"name,omitempty"`
	// What was on our screen before us: the clock face and the set of screens it
	// belongs to. Remembered on the first run and given back on uninstall —
	// otherwise the screen is left with a dead picture once the bridge is gone.
	PrevClockID      int `json:"prev_clock_id,omitempty"`
	PrevIndependence int `json:"prev_independence,omitempty"`
}

// errNoConfig — the panel was never turned on. Told apart from a broken file:
// that one has to be reported instead of quietly starting from the defaults.
var errNoConfig = errors.New(i18n.T("the panel is not turned on — claudestatus divoom on"))

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
			return cfg, errNoConfig
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

// update re-reads the file and lets change touch only the fields it means to.
// The bridge lives for hours with a copy of the config in memory, while the
// human keeps giving orders — `screen`, `off` — through another process. Saving
// the whole copy would put a stale answer back over a fresh one; the last word
// belongs to whoever spoke last.
func update(change func(*config)) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	change(&cfg)
	return cfg.save()
}

func (c config) enabled() bool {
	return c.On == nil || *c.On
}

func (c *config) setOn(on bool) {
	c.On = &on
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

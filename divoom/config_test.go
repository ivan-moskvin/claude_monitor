package divoom

import (
	"errors"
	"os"
	"testing"

	"github.com/ivan-moskvin/claude_monitor/internal/testenv"
)

// No config means the panel was never turned on — told apart from a broken one,
// which has to be reported instead of quietly starting from the defaults.
func TestLoadConfigWithoutAFile(t *testing.T) {
	testenv.Home(t)

	cfg, err := loadConfig()
	if !errors.Is(err, errNoConfig) {
		t.Fatalf("loadConfig() error = %v; want errNoConfig", err)
	}
	if cfg.LcdIndex != 4 || cfg.Port != defaultPort {
		t.Errorf("loadConfig() = %+v; want the defaults", cfg)
	}
}

func TestLoadConfigBroken(t *testing.T) {
	testenv.Home(t)
	writeConfig(t, "{not json")

	if _, err := loadConfig(); err == nil || errors.Is(err, errNoConfig) {
		t.Fatalf("loadConfig() error = %v; want a complaint about the file", err)
	}
}

func TestConfigRoundTrip(t *testing.T) {
	testenv.Home(t)

	written := config{IP: "192.168.0.5", LcdIndex: 2, Port: 9000, DeviceID: 7, MAC: "aa:bb", Name: "Times Gate"}
	if err := written.save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	read, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if read != written {
		t.Errorf("loadConfig() = %+v; want %+v", read, written)
	}
}

// A config written before the port existed still has to bring the bridge up.
func TestLoadConfigFillsInThePort(t *testing.T) {
	testenv.Home(t)
	writeConfig(t, `{"ip":"192.168.0.5"}`)

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.Port != defaultPort {
		t.Errorf("Port = %d; want the default %d", cfg.Port, defaultPort)
	}
}

// `off` clears the flag but keeps the file: the chosen device and screen have
// to survive it, or every `on` would start over from the defaults.
func TestConfigEnabled(t *testing.T) {
	on, off := true, false
	cases := []struct {
		name string
		cfg  config
		want bool
	}{
		// A config written before the flag existed carries no value and counts
		// as on.
		{"written before the flag existed", config{}, true},
		{"turned on", config{On: &on}, true},
		{"turned off", config{On: &off}, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.cfg.enabled(); got != c.want {
				t.Errorf("enabled() = %t; want %t", got, c.want)
			}
		})
	}

	var cfg config
	cfg.setOn(false)
	if cfg.enabled() {
		t.Error("setOn(false) left the panel on")
	}
	cfg.setOn(true)
	if !cfg.enabled() {
		t.Error("setOn(true) left the panel off")
	}
}

// The bridge lives for hours with a copy of the config in memory while the
// human keeps giving orders through another process: update has to re-read the
// file and touch only the field it means to.
func TestUpdateKeepsWhatItDidNotChange(t *testing.T) {
	testenv.Home(t)
	if err := (config{IP: "192.168.0.5", LcdIndex: 4, Port: defaultPort}).save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Somebody else moves the panel to another screen meanwhile.
	if err := update(func(cfg *config) { cfg.LcdIndex = 1 }); err != nil {
		t.Fatalf("update: %v", err)
	}
	// And we write down the address we have just found.
	if err := update(func(cfg *config) { cfg.IP = "192.168.0.9" }); err != nil {
		t.Fatalf("update: %v", err)
	}

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.IP != "192.168.0.9" {
		t.Errorf("IP = %q; want the address written last", cfg.IP)
	}
	if cfg.LcdIndex != 1 {
		t.Errorf("LcdIndex = %d; want the screen the human chose to survive", cfg.LcdIndex)
	}
}

func TestUpdateWithoutAConfig(t *testing.T) {
	testenv.Home(t)

	// Nothing to update: the panel was never turned on.
	if err := update(func(cfg *config) { cfg.IP = "192.168.0.9" }); !errors.Is(err, errNoConfig) {
		t.Errorf("update() error = %v; want errNoConfig", err)
	}
}

func writeConfig(t *testing.T, content string) {
	t.Helper()

	path, err := configPath()
	if err != nil {
		t.Fatalf("configPath: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing the config: %v", err)
	}
}

package divoom

import (
	"bytes"
	"image/gif"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ivan-moskvin/claudestatus/i18n"
	"github.com/ivan-moskvin/claudestatus/internal/testenv"
)

func TestRunHelp(t *testing.T) {
	for _, arg := range []string{"help", "--help", "-h"} {
		printed := testenv.CaptureStdout(t, func() {
			if err := Run([]string{arg}); err != nil {
				t.Errorf("Run(%q): %v", arg, err)
			}
		})
		if !strings.Contains(printed, "claudestatus divoom") {
			t.Errorf("Run(%q) printed %q; want the usage", arg, printed)
		}
	}
}

func TestRunUnknownCommand(t *testing.T) {
	err := Run([]string{"fly"})
	if err == nil {
		t.Fatal("an unknown command was accepted")
	}
	// The usage comes with the complaint: the human typed something close.
	if !strings.Contains(err.Error(), "fly") || !strings.Contains(err.Error(), "claudestatus divoom") {
		t.Errorf("Run(\"fly\") = %v; want the command and the usage", err)
	}
}

// preview draws the same frame the device would get, without touching the
// device — that is what makes a panel checkable at all.
func TestRunPreview(t *testing.T) {
	testenv.Home(t)
	path := filepath.Join(t.TempDir(), "panel.gif")

	if err := Run([]string{"preview", path}); err != nil {
		t.Fatalf("Run(preview): %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the frame was not written: %v", err)
	}
	img, err := gif.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("the frame does not decode as a GIF: %v", err)
	}
	if img.Bounds().Dx() != panelSize {
		t.Errorf("the frame is %d pixels wide; want %d", img.Bounds().Dx(), panelSize)
	}
}

func TestRunPreviewWithoutAFile(t *testing.T) {
	if err := Run([]string{"preview"}); err == nil {
		t.Error("preview with nowhere to write reported success")
	}
}

// Every command that needs the panel turned on says so instead of starting from
// the defaults on a machine that never ran `divoom on`.
func TestCommandsWithoutAConfig(t *testing.T) {
	cases := map[string][]string{
		"the bridge": nil,
		"once":       {"once"},
		"screen":     {"screen"},
	}

	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			testenv.Home(t)
			if err := Run(args); err == nil {
				t.Error("the command ran with no panel configured")
			}
		})
	}

	// Turning off what was never on is not a failure: uninstall runs it on every
	// machine.
	t.Run("off", func(t *testing.T) {
		testenv.Home(t)
		if err := Run([]string{"off"}); err != nil {
			t.Errorf("Run(off): %v", err)
		}
	})
}

func TestScreenShowsTheCurrentOne(t *testing.T) {
	testenv.Home(t)
	saveConfig(t, config{IP: unreachableDevice, LcdIndex: 4, Port: defaultPort})

	printed := testenv.CaptureStdout(t, func() {
		if err := screen(nil); err != nil {
			t.Fatalf("screen: %v", err)
		}
	})
	// The screens are numbered from zero inside and from one for the human, the
	// way the Divoom app labels them.
	if !strings.Contains(printed, "5") {
		t.Errorf("screen() printed %q; want screen 5 of 5", printed)
	}
}

func TestScreenMoves(t *testing.T) {
	testenv.Home(t)
	saveConfig(t, config{IP: unreachableDevice, LcdIndex: 4, Port: defaultPort, PrevClockID: 61, PrevIndependence: 1})

	testenv.CaptureStdout(t, func() {
		if err := screen([]string{"2"}); err != nil {
			t.Fatalf("screen: %v", err)
		}
	})

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.LcdIndex != 1 {
		t.Errorf("LcdIndex = %d; want the second screen", cfg.LcdIndex)
	}
	// What was on the new screen we do not know yet — the bridge remembers it at
	// startup, and the clock face of the old one must not be put back there.
	if cfg.PrevClockID != 0 || cfg.PrevIndependence != 0 {
		t.Errorf("the previous clock face was carried over: %d/%d", cfg.PrevClockID, cfg.PrevIndependence)
	}
}

func TestScreenStaysWhereItIs(t *testing.T) {
	testenv.Home(t)
	saveConfig(t, config{IP: unreachableDevice, LcdIndex: 1, Port: defaultPort, PrevClockID: 61})

	testenv.CaptureStdout(t, func() {
		if err := screen([]string{"2"}); err != nil {
			t.Fatalf("screen: %v", err)
		}
	})

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	// Nothing moved, so the remembered clock face is still the right one.
	if cfg.PrevClockID != 61 {
		t.Errorf("PrevClockID = %d; want the screen left alone", cfg.PrevClockID)
	}
}

func TestScreenRejectsANonScreen(t *testing.T) {
	for _, arg := range []string{"0", "6", "-1", "two", ""} {
		t.Run(arg, func(t *testing.T) {
			testenv.Home(t)
			saveConfig(t, config{IP: unreachableDevice, LcdIndex: 4, Port: defaultPort})

			if err := screen([]string{arg}); err == nil {
				t.Errorf("screen(%q) was accepted", arg)
			}
		})
	}
}

// With twenty devices around, guessing would light up somebody else's screen.
func TestChoose(t *testing.T) {
	list := []found{
		{ip: "192.168.0.5", name: "Times Gate", id: 7},
		{ip: "192.168.0.6", name: "Pixoo", id: 9},
	}

	t.Run("named on the command line", func(t *testing.T) {
		picked, err := choose(list, config{}, []string{"2"})
		if err != nil || picked.ip != "192.168.0.6" {
			t.Errorf("choose() = %+v, %v; want the second device", picked, err)
		}
	})

	t.Run("the one chosen before", func(t *testing.T) {
		picked, err := choose(list, config{DeviceID: 7}, nil)
		if err != nil || picked.ip != "192.168.0.5" {
			t.Errorf("choose() = %+v, %v; want the remembered device", picked, err)
		}
	})

	t.Run("the only one around", func(t *testing.T) {
		picked, err := choose(list[:1], config{}, nil)
		if err != nil || picked.ip != "192.168.0.5" {
			t.Errorf("choose() = %+v, %v; want the only device", picked, err)
		}
	})

	// A number nobody can honour is a question back, not a guess.
	for _, arg := range []string{"0", "3", "second"} {
		t.Run("not a device: "+arg, func(t *testing.T) {
			testenv.CaptureStdout(t, func() {
				if _, err := choose(list, config{}, []string{arg}); err == nil {
					t.Errorf("choose(%q) picked a device anyway", arg)
				}
			})
		})
	}

	// Nothing to go by and nobody at the keyboard: a test run is not a terminal,
	// so the answer is a command to type rather than a prompt into the void.
	t.Run("more than one and nobody to ask", func(t *testing.T) {
		printed := testenv.CaptureStdout(t, func() {
			if _, err := choose(list, config{}, nil); err == nil {
				t.Error("a device was chosen with nothing to choose by")
			}
		})
		if !strings.Contains(printed, "192.168.0.5") || !strings.Contains(printed, "192.168.0.6") {
			t.Errorf("the devices were not listed: %q", printed)
		}
	})
}

func TestPrintDevices(t *testing.T) {
	printed := testenv.CaptureStdout(t, func() {
		printDevices([]found{{ip: "192.168.0.5", name: "Times Gate"}, {ip: "192.168.0.6"}})
	})

	if !strings.Contains(printed, "1. Times Gate — 192.168.0.5") {
		t.Errorf("the named device is listed as %q", printed)
	}
	// A device the scan found on its own still needs a name to point at.
	if !strings.Contains(printed, "2. "+i18n.T("Divoom device")+" — 192.168.0.6") {
		t.Errorf("the unnamed device is listed as %q", printed)
	}
}

// An address nothing listens on: the commands below give the screen its clock
// face back, and a test must not knock on a real device — least of all on
// whatever sits at that address in the network it happens to run in.
const unreachableDevice = "127.0.0.1:1"

func saveConfig(t *testing.T, cfg config) {
	t.Helper()

	if err := cfg.save(); err != nil {
		t.Fatalf("saving the config: %v", err)
	}
}

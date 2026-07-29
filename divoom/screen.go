package divoom

import (
	"fmt"
	"strconv"

	"github.com/ivan-moskvin/claude_monitor/i18n"
)

// A Times Gate has five screens. Internally they are numbered from zero, but
// the human is shown 1–5: that is how they are labelled in the Divoom app.
const screenCount = 5

// screen shows or changes the screen the panel occupies. Changing it is more
// than editing the config: the previous screen has to get its clock face back,
// otherwise a dead picture stays on it, and the bridge has to move to the new
// one.
func screen(args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	if len(args) == 0 {
		fmt.Printf(i18n.T("The panel is on screen %d of %d\n"), cfg.LcdIndex+1, screenCount)
		return nil
	}

	number, err := strconv.Atoi(args[0])
	if err != nil || number < 1 || number > screenCount {
		return fmt.Errorf(i18n.T("the screen is a number from 1 to %d, not %q"), screenCount, args[0])
	}

	if number-1 == cfg.LcdIndex {
		fmt.Printf(i18n.T("The panel is on screen %d already\n"), number)
		return nil
	}

	// The bridge holds the screen, so the old one is released first — Stop waits
	// until the bridge has put the clock face back there.
	wasRunning := running()
	if wasRunning {
		Stop()
	}

	cfg.LcdIndex = number - 1
	// What was on the new screen we do not know yet — the bridge remembers it at
	// startup.
	cfg.PrevClockID, cfg.PrevIndependence = 0, 0
	if err := cfg.save(); err != nil {
		return err
	}

	fmt.Printf(i18n.T("The panel moved to screen %d\n"), number)
	if wasRunning {
		EnsureRunning()
	}
	return nil
}

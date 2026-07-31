package divoom

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ivan-moskvin/claudestatus/i18n"
)

// on looks for the Divoom devices on the network and turns the panel on. The
// device is searched for every time: its address comes from DHCP and does not
// have to be the one of the last run.
func on(args []string) error {
	cfg, err := loadConfig()
	if err != nil && !errors.Is(err, errNoConfig) {
		// A broken config is worth a word: silently starting from the defaults
		// would throw away the chosen screen.
		return err
	}

	list, err := devicesOnNetwork()
	if err != nil {
		return err
	}

	picked, err := choose(list, cfg, args)
	if err != nil {
		return err
	}

	cfg.IP, cfg.DeviceID, cfg.MAC, cfg.Name = picked.ip, picked.id, picked.mac, picked.name
	cfg.setOn(true)
	if cfg.Port == 0 {
		cfg.Port = defaultPort
	}
	if err := cfg.save(); err != nil {
		return err
	}

	fmt.Printf(i18n.T("Device %s: %s\n"), picked.label(), picked.ip)

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

// choose picks the device to hand the panel to: the number given on the command
// line wins, then the one chosen before, and a single device on the network
// needs no choosing at all. Everything else is a question to the human — with
// twenty devices around, guessing would light up somebody else's screen.
func choose(list []found, cfg config, args []string) (found, error) {
	if len(args) > 0 {
		number, err := strconv.Atoi(args[0])
		if err != nil || number < 1 || number > len(list) {
			printDevices(list)
			return found{}, fmt.Errorf(i18n.T("the device is a number from 1 to %d, not %q"), len(list), args[0])
		}
		return list[number-1], nil
	}

	if picked, ok := match(list, cfg); ok {
		return picked, nil
	}
	if len(list) == 1 {
		return list[0], nil
	}

	printDevices(list)
	if !interactive() {
		return found{}, errors.New(i18n.T("there is more than one device — name the number: claudestatus divoom on N"))
	}

	fmt.Printf(i18n.T("Which one gets the panel? 1–%d: "), len(list))
	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return found{}, err
	}
	number, err := strconv.Atoi(strings.TrimSpace(answer))
	if err != nil || number < 1 || number > len(list) {
		return found{}, fmt.Errorf(i18n.T("the device is a number from 1 to %d, not %q"), len(list), strings.TrimSpace(answer))
	}
	return list[number-1], nil
}

func printDevices(list []found) {
	fmt.Println(i18n.T("Divoom devices on the network:"))
	for i, entry := range list {
		fmt.Printf("  %d. %s — %s\n", i+1, entry.label(), entry.ip)
	}
}

func off() error {
	if running() {
		Stop()
	} else {
		restore()
	}

	// The config stays: the device and the screen chosen by the human are the
	// settings of the panel, not the state of a running bridge.
	cfg, err := loadConfig()
	if err != nil {
		if errors.Is(err, errNoConfig) {
			return nil
		}
		return err
	}
	cfg.setOn(false)
	if err := cfg.save(); err != nil {
		return err
	}

	// A bridge left over from an update or a second copy notices the cleared
	// flag only on its next tick, and until then it keeps drawing. Give it that
	// moment and put the clock face back once more.
	time.Sleep(pollInterval + time.Second)
	restore()

	fmt.Println(i18n.T("The panel is off, the screen got its clock face back"))
	return nil
}

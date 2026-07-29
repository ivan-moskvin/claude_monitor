//go:build darwin

package i18n

import (
	"os/exec"
	"strings"
	"time"
)

// systemLang asks macOS itself when the environment says nothing: a terminal
// there usually leaves LANG empty or sets C.UTF-8, while the language a person
// picked in System Settings lives in the global preferences.
func systemLang() Lang {
	if lang, ok := fromEnv(); ok {
		return lang
	}
	if lang, ok := parse(appleLocale()); ok {
		return lang
	}
	return EN
}

func appleLocale() string {
	done := make(chan string, 1)
	go func() {
		out, err := exec.Command("defaults", "read", "-g", "AppleLocale").Output()
		if err != nil {
			done <- ""
			return
		}
		done <- strings.TrimSpace(string(out))
	}()

	// The status line must not wait on a subprocess: English is a fine answer
	// if the system is slow to reply.
	select {
	case locale := <-done:
		return locale
	case <-time.After(300 * time.Millisecond):
		return ""
	}
}

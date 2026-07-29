//go:build !windows && !darwin

package i18n

// Asking the system for its language is done differently on every platform, so
// it lives in files behind build tags: the Windows call needs kernel32, which
// does not exist elsewhere, and cross-compiling the release binaries would fail
// on it.

// systemLang trusts the environment: on Unix the shell exports the locale, and
// there is nothing else to ask.
func systemLang() Lang {
	if lang, ok := fromEnv(); ok {
		return lang
	}
	return EN
}

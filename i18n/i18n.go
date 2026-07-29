// Package i18n holds every string the user is meant to read.
//
// English is the source language: T is called with the English text itself, so
// a missing translation still prints a readable message instead of a bare key,
// and the call site stays legible without looking the key up. A language is
// added by one more catalog file plus one line in catalogs — nothing branches
// on the language outside this package.
package i18n

import (
	"os"
	"strings"
)

// Lang is a language tag, the same spelling CLAUDESTATUS_LANG accepts.
type Lang string

const (
	EN Lang = "en"
	RU Lang = "ru"
)

// catalogs maps English source text to its translation, one map per language.
// English needs no entry: it is what the call sites already say.
var catalogs = map[Lang]map[string]string{
	RU: ru,
}

// The language is resolved once per process: it cannot change while we run, and
// the status line is called often enough not to redo the lookup every time.
var current = detect()

// Current reports the language chosen for this process.
func Current() Lang { return current }

// T translates the English source text, returning it unchanged when the catalog
// has nothing for it.
func T(text string) string {
	if translated, ok := catalogs[current][text]; ok {
		return translated
	}
	return text
}

// detect resolves the language: an explicit override wins over the system, so a
// Russian desktop can still be checked in English while debugging.
func detect() Lang {
	if lang, ok := parse(os.Getenv("CLAUDESTATUS_LANG")); ok {
		return lang
	}
	return systemLang()
}

// fromEnv reads the POSIX locale variables in their usual precedence.
func fromEnv() (Lang, bool) {
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if lang, ok := parse(os.Getenv(name)); ok {
			return lang, true
		}
	}
	return EN, false
}

// parse takes the language out of a locale name ("ru_RU.UTF-8", "ru", "en_US").
// Anything we have no catalog for is English.
func parse(value string) (Lang, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return EN, false
	}
	if strings.HasPrefix(value, string(RU)) {
		return RU, true
	}
	return EN, true
}

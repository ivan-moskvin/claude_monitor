//go:build windows

package i18n

import "syscall"

// A LANGID keeps the sublanguage in its upper bits, so every Russian locale —
// 0x0419 among them — shares the same primary language in the lower ten.
const (
	langPrimaryMask = 0x3ff
	langRussian     = 0x19
)

// systemLang asks Windows for the UI language. The POSIX variables still come
// first — a shell that sets them means it — but they are normally empty here,
// so GetUserDefaultUILanguage is the only thing that answers.
func systemLang() Lang {
	if lang, ok := fromEnv(); ok {
		return lang
	}

	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("GetUserDefaultUILanguage")
	if proc.Find() != nil {
		return EN
	}
	id, _, _ := proc.Call()
	if uint32(id)&langPrimaryMask == langRussian {
		return RU
	}
	return EN
}

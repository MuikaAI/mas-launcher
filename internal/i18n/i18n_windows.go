//go:build windows

package i18n

import (
	"syscall"
)

var (
	kernel32                 = syscall.NewLazyDLL("kernel32.dll")
	getUserDefaultUILanguage = kernel32.NewProc("GetUserDefaultUILanguage")
)

// primaryLangID returns the primary language id from a Windows LANGID (low
// 10 bits carry the primary language, bits 10-15 the sub-language).
func primaryLangID(langID uint32) uint32 {
	return langID & 0x3ff
}

// systemUILang reports the user's UI language as a normalized lang id
// ("zh" / "en") when it matches a supported language, or "" otherwise.
func systemUILang() string {
	r, _, _ := getUserDefaultUILanguage.Call()
	switch primaryLangID(uint32(r)) {
	case 0x04: // LANG_CHINESE
		return "zh"
	case 0x09: // LANG_ENGLISH
		return "en"
	}
	return ""
}

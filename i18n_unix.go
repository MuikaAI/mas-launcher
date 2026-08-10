//go:build !windows

package main

// systemUILang is unavailable on non-Windows platforms; language detection
// relies on the MUIKA_LANG / LC_ALL / LANG environment variables.
func systemUILang() string {
	return ""
}

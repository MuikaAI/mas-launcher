package i18n

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestNormalizeLang(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{"zh", "zh"},
		{"zh-CN", "zh"},
		{"zh_CN", "zh"},
		{"zh_CN.UTF-8", "zh"},
		{"ZH_CN.UTF-8", "zh"},
		{"en", "en"},
		{"en_US", "en"},
		{"en-US.utf8", "en"},
		{"fr", ""},
		{"fr_FR", ""},
		{"", ""},
	} {
		if got := normalizeLang(tc.in); got != tc.want {
			t.Errorf("normalizeLang(%q) = %q want %q", tc.in, got, tc.want)
		}
	}
}

func TestDetectLangPrecedence(t *testing.T) {
	t.Setenv("MUIKA_LANG", "zh_CN")
	t.Setenv("LC_ALL", "en_US")
	t.Setenv("LANG", "en_US")
	if got := detectLang(); got != "zh" {
		t.Errorf("MUIKA_LANG should win, got %q", got)
	}
	t.Setenv("MUIKA_LANG", "")
	t.Setenv("LC_ALL", "zh_CN.UTF-8")
	t.Setenv("LANG", "en_US")
	if got := detectLang(); got != "zh" {
		t.Errorf("LC_ALL should win over LANG, got %q", got)
	}
	t.Setenv("MUIKA_LANG", "")
	t.Setenv("LC_ALL", "")
	t.Setenv("LANG", "en_US")
	if got := detectLang(); got != "en" {
		t.Errorf("LANG fallback, got %q", got)
	}
}

func TestT(t *testing.T) {
	t.Setenv("MUIKA_LANG", "zh")
	t.Setenv("LC_ALL", "")
	t.Setenv("LANG", "")
	if got := T("Instance %s stopped.\n", "default"); got != "实例 default 已停止。\n" {
		t.Errorf("zh translation failed, got %q", got)
	}
	if got := T("no such key"); got != "no such key" {
		t.Errorf("missing key should fall back to key, got %q", got)
	}

	t.Setenv("MUIKA_LANG", "en")
	if got := T("Instance %s stopped.\n", "default"); got != "Instance default stopped.\n" {
		t.Errorf("en fallback should use the key, got %q", got)
	}
}

var verbRe = regexp.MustCompile(`%[+#\-0-9. ]*[vTtbcdoOqxXUeEfFgGspw%]`)

// TestZhVerbConsistency ensures every translation keeps the same printf verb
// sequence as its key, so translations can never shift format arguments.
func TestZhVerbConsistency(t *testing.T) {
	for key, tr := range zhMessages {
		keys := verbRe.FindAllString(key, -1)
		trs := verbRe.FindAllString(tr, -1)
		if len(keys) != len(trs) {
			t.Errorf("verb count mismatch for %q:\n  key verbs:   %v\n  tr  verbs:   %v", key, keys, trs)
			continue
		}
		for i := range keys {
			if keys[i] != trs[i] {
				t.Errorf("verb %d mismatch for %q: %q vs %q", i, key, keys[i], trs[i])
			}
		}
	}
}

var (
	quotedKeyRe = regexp.MustCompile(`T\(\s*"((?:[^"\\]|\\.)*)"\s*\)`)
	rawKeyRe    = regexp.MustCompile("T\\(\\s*`((?:[^`])*)`\\s*\\)")
)

// TestCatalogCompleteness ensures every T("...") / T(`...`) literal call site
// in the packages that use the catalog has a matching zhMessages entry, so a
// mistyped key can never silently stay English instead of raising an error.
// It scans the sibling caller packages (the core command layer plus the
// models and repository packages, which localize their own errors).
func TestCatalogCompleteness(t *testing.T) {
	checked := 0
	for _, dir := range []string{"../core", "../models", "../repository"} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			b, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			src := string(b)
			for _, m := range quotedKeyRe.FindAllStringSubmatch(src, -1) {
				key, err := strconv.Unquote(`"` + m[1] + `"`)
				if err != nil {
					t.Fatalf("%s: cannot unquote %q: %v", name, m[1], err)
				}
				checked++
				if _, ok := zhMessages[key]; !ok {
					t.Errorf("%s: missing catalog entry for key %q", filepath.Join(dir, name), key)
				}
			}
			for _, m := range rawKeyRe.FindAllStringSubmatch(src, -1) {
				key, err := strconv.Unquote("`" + m[1] + "`")
				if err != nil {
					t.Fatalf("%s: cannot unquote raw key: %v", name, err)
				}
				checked++
				if _, ok := zhMessages[key]; !ok {
					t.Errorf("%s: missing catalog entry for raw key", filepath.Join(dir, name))
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no T() literals found; completeness test is not scanning anything")
	}
}

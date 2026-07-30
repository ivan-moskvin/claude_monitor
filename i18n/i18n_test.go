package i18n

import (
	"regexp"
	"testing"
)

func TestParse(t *testing.T) {
	cases := []struct {
		value string
		want  Lang
		known bool
	}{
		{"ru_RU.UTF-8", RU, true},
		{"ru", RU, true},
		{"RU", RU, true},
		{" ru_RU.UTF-8 ", RU, true},
		{"ru-RU", RU, true},
		{"en_US.UTF-8", EN, true},
		{"de_DE.UTF-8", EN, true},
		// An empty value is not a choice: the next source in the chain gets asked.
		{"", EN, false},
		{"   ", EN, false},
	}

	for _, c := range cases {
		lang, known := parse(c.value)
		if lang != c.want || known != c.known {
			t.Errorf("parse(%q) = %q, %t; want %q, %t", c.value, lang, known, c.want, c.known)
		}
	}
}

func TestFromEnvPrecedence(t *testing.T) {
	cases := []struct {
		name                    string
		lcAll, lcMessages, lang string
		want                    Lang
		known                   bool
	}{
		{name: "nothing set", want: EN, known: false},
		{name: "LANG alone", lang: "ru_RU.UTF-8", want: RU, known: true},
		{name: "LC_MESSAGES over LANG", lcMessages: "ru_RU.UTF-8", lang: "en_US.UTF-8", want: RU, known: true},
		{name: "LC_ALL over the rest", lcAll: "ru_RU.UTF-8", lcMessages: "en_US.UTF-8", lang: "en_US.UTF-8", want: RU, known: true},
		// A locale we have no catalog for is an answer all the same: the user
		// asked for German, not for the Russian sitting further down the chain.
		{name: "unknown locale stops the chain", lcAll: "de_DE.UTF-8", lang: "ru_RU.UTF-8", want: EN, known: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("LC_ALL", c.lcAll)
			t.Setenv("LC_MESSAGES", c.lcMessages)
			t.Setenv("LANG", c.lang)

			lang, known := fromEnv()
			if lang != c.want || known != c.known {
				t.Errorf("fromEnv() = %q, %t; want %q, %t", lang, known, c.want, c.known)
			}
		})
	}
}

func TestInFallsBackToTheSourceText(t *testing.T) {
	if got := In(RU, "reset"); got != "сброс" {
		t.Errorf(`In(RU, "reset") = %q; want "сброс"`, got)
	}
	// English has no catalog: the call site already says it.
	if got := In(EN, "reset"); got != "reset" {
		t.Errorf(`In(EN, "reset") = %q; want "reset"`, got)
	}
	// An untranslated string prints as it stands — never as a bare key.
	const missing = "nothing in any catalog says this"
	for _, lang := range Langs() {
		if got := In(lang, missing); got != missing {
			t.Errorf("In(%q, missing) = %q; want the source text", lang, got)
		}
	}
}

func TestLangsCoversEveryCatalog(t *testing.T) {
	langs := Langs()
	if len(langs) == 0 || langs[0] != EN {
		t.Fatalf("Langs() = %v; want English first", langs)
	}

	listed := make(map[Lang]bool, len(langs))
	for _, lang := range langs {
		if listed[lang] {
			t.Errorf("Langs() lists %q twice", lang)
		}
		listed[lang] = true
	}
	for lang := range catalogs {
		if !listed[lang] {
			t.Errorf("catalog %q is missing from Langs()", lang)
		}
	}
}

// The format verbs are part of the key: a translation that drops one or swaps
// two around prints garbage — %!d(MISSING) or the arguments in the wrong slots.
func TestCatalogsKeepTheFormatVerbs(t *testing.T) {
	for lang, catalog := range catalogs {
		for source, translation := range catalog {
			want, got := verbs(source), verbs(translation)
			if len(want) != len(got) {
				t.Errorf("%s: %q has verbs %v, the translation has %v", lang, short(source), want, got)
				continue
			}
			for i := range want {
				if want[i] != got[i] {
					t.Errorf("%s: %q has verbs %v, the translation has %v", lang, short(source), want, got)
					break
				}
			}
		}
	}
}

func TestCatalogsHaveNoEmptyTranslations(t *testing.T) {
	for lang, catalog := range catalogs {
		for source, translation := range catalog {
			if translation == "" {
				t.Errorf("%s: %q translates to nothing", lang, short(source))
			}
		}
	}
}

// A translation that ends up as its own key is a leftover: either the string
// needs no catalog entry, or the entry was never filled in.
func TestCatalogsTranslateSomething(t *testing.T) {
	for lang, catalog := range catalogs {
		for source, translation := range catalog {
			if source == translation {
				t.Errorf("%s: %q is left untranslated — drop the entry or fill it in", lang, short(source))
			}
		}
	}
}

// verbFormat matches a printf verb without swallowing "%%", which is a literal
// percent sign and takes no argument.
var verbFormat = regexp.MustCompile(`%[#+\-  0']*[0-9]*(?:\.[0-9]+)?[a-zA-Z%]`)

func verbs(text string) []string {
	var found []string
	for _, verb := range verbFormat.FindAllString(text, -1) {
		if verb == "%%" {
			continue
		}
		found = append(found, verb)
	}
	return found
}

// short keeps the failure message readable: some keys are whole help screens.
func short(text string) string {
	runes := []rune(text)
	if len(runes) <= 40 {
		return text
	}
	return string(runes[:40]) + "…"
}

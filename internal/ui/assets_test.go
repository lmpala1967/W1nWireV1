package ui

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The interface is written by hand in JavaScript, so nothing but a test stops
// a translation key from disappearing or a button from losing its handler.
// The checks below read the assets as text and compare them with each other.

// readAsset returns the content of a file of the embedded interface.
func readAsset(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile("assets/" + name)
	if err != nil {
		t.Fatalf("the asset %s is missing: %v", name, err)
	}
	return string(data)
}

// dictionaryKeys returns the keys of one language of the I18N dictionary.
func dictionaryKeys(t *testing.T, script, language string) map[string]bool {
	t.Helper()
	marker := "\n  " + language + ": {"
	start := strings.Index(script, marker)
	if start < 0 {
		t.Fatalf("the %s dictionary was not found", language)
	}
	rest := script[start+len(marker):]
	// The last block of the object ends with a brace that is not followed by
	// a comma, so both endings have to be accepted.
	end := regexp.MustCompile(`\n  \}(,|\r?\n)`).FindStringIndex(rest)
	if end == nil {
		t.Fatalf("the %s dictionary is not closed", language)
	}
	body := rest[:end[0]]
	re := regexp.MustCompile(`'([a-zA-Z][a-zA-Z0-9_.]*)'\s*:`)
	keys := map[string]bool{}
	for _, match := range re.FindAllStringSubmatch(body, -1) {
		keys[match[1]] = true
	}
	if len(keys) == 0 {
		t.Fatalf("the %s dictionary is empty", language)
	}
	return keys
}

// usedKeys returns the translation keys the interface asks for.
//
// The pattern requires a word boundary before the t, so that a call like
// querySelector('div') is not mistaken for a translation.
func usedKeys(script string) map[string]bool {
	re := regexp.MustCompile(`\bt\(\s*'([a-zA-Z][a-zA-Z0-9_.]*)'`)
	keys := map[string]bool{}
	for _, match := range re.FindAllStringSubmatch(script, -1) {
		keys[match[1]] = true
	}
	return keys
}

// dynamicKeyPrefixes are the prefixes of the keys that are built at run
// time, for example t('state.' + state). They are checked by prefix.
var dynamicKeyPrefixes = []string{
	"state.",
	"nav.",
	"logs.level.",
}

// TestTranslationsAreComplete checks that every key the interface uses
// exists in both languages, and that the two languages have the same keys.
func TestTranslationsAreComplete(t *testing.T) {
	script := readAsset(t, "app.js")
	russian := dictionaryKeys(t, script, "ru")
	english := dictionaryKeys(t, script, "en")

	for key := range russian {
		if !english[key] {
			t.Errorf("the key %q is missing in the English dictionary", key)
		}
	}
	for key := range english {
		if !russian[key] {
			t.Errorf("the key %q is missing in the Russian dictionary", key)
		}
	}

	for key := range usedKeys(script) {
		if strings.HasSuffix(key, ".") {
			// The rest of the key is built at run time.
			continue
		}
		if !russian[key] {
			t.Errorf("the key %q is used but not defined", key)
		}
	}

	// Every state must have a label in both languages.
	for _, state := range []string{"disconnected", "connecting", "connected", "reconnecting", "disconnecting", "error"} {
		if !russian["state."+state] {
			t.Errorf("the state %q has no label", state)
		}
		if !english["state."+state] {
			t.Errorf("the state %q has no English label", state)
		}
	}
	// Every screen must have a label in the navigation.
	for _, view := range []string{"connect", "servers", "settings", "logs", "about"} {
		if !russian["nav."+view] {
			t.Errorf("the screen %q has no label", view)
		}
		if !english["nav."+view] {
			t.Errorf("the screen %q has no English label", view)
		}
	}
	// Every log level must have a label.
	for _, level := range []string{"info", "warn", "error"} {
		if !russian["logs.level."+level] {
			t.Errorf("the log level %q has no label", level)
		}
		if !english["logs.level."+level] {
			t.Errorf("the log level %q has no English label", level)
		}
	}
}

// TestTranslationsHaveNoDeadKeys checks that no key of the dictionary is
// left over. A dead key is a translation nobody ever reads, and it hides
// the fact that a piece of the interface was never wired up.
func TestTranslationsHaveNoDeadKeys(t *testing.T) {
	script := readAsset(t, "app.js")
	used := usedKeys(script)

	// The keys that are built at run time are covered by their prefix.
	for _, prefix := range dynamicKeyPrefixes {
		for key := range dictionaryKeys(t, script, "ru") {
			if strings.HasPrefix(key, prefix) {
				used[key] = true
			}
		}
	}
	// The navigation labels are read through the data-i18n attributes.
	re := regexp.MustCompile(`data-i18n="([^"]+)"`)
	for _, match := range re.FindAllStringSubmatch(script, -1) {
		used[match[1]] = true
	}
	// The html file has a few of its own.
	html := readAsset(t, "index.html")
	for _, match := range re.FindAllStringSubmatch(html, -1) {
		used[match[1]] = true
	}

	dead := []string{}
	for key := range dictionaryKeys(t, script, "ru") {
		if !used[key] {
			dead = append(dead, key)
		}
	}
	sort.Strings(dead)
	for _, key := range dead {
		t.Errorf("the key %q is never used", key)
	}
}

// TestEveryActionHasACase checks that every button of the interface has a
// handler. A button whose action has no case does nothing at all, and the
// interface looks like it works.
func TestEveryActionHasACase(t *testing.T) {
	script := readAsset(t, "app.js")
	html := readAsset(t, "index.html")

	actions := map[string]bool{}
	re := regexp.MustCompile(`data-action="([a-zA-Z-]+)"`)
	for _, match := range re.FindAllStringSubmatch(script, -1) {
		actions[match[1]] = true
	}
	for _, match := range re.FindAllStringSubmatch(html, -1) {
		actions[match[1]] = true
	}
	if len(actions) == 0 {
		t.Fatal("no action was found in the interface")
	}

	cases := map[string]bool{}
	caseRe := regexp.MustCompile(`case '([a-zA-Z-]+)':`)
	for _, match := range caseRe.FindAllStringSubmatch(script, -1) {
		cases[match[1]] = true
	}

	missing := []string{}
	for action := range actions {
		// The backdrop of a modal is handled before the dispatcher, it
		// closes the modal when it is clicked.
		if action == "modal-backdrop" {
			continue
		}
		if !cases[action] {
			missing = append(missing, action)
		}
	}
	sort.Strings(missing)
	for _, action := range missing {
		t.Errorf("the action %q has no handler", action)
	}
}

// TestSwitchAttributesMatchTheDispatcher guards the switches of the
// settings screen. They are buttons, so they travel through data-action and
// not through a data attribute of their own: a mismatch leaves them dead.
func TestSwitchAttributesMatchTheDispatcher(t *testing.T) {
	script := readAsset(t, "app.js")
	if strings.Contains(script, "data-toggle-setting") {
		t.Error("the switches still use data-toggle-setting, the dispatcher looks for data-action")
	}
	if !strings.Contains(script, `data-action="toggle-setting"`) {
		t.Error("no switch uses the toggle-setting action")
	}
	if !strings.Contains(script, "data-setting-key") {
		t.Error("the switch does not carry the name of the setting it changes")
	}
}

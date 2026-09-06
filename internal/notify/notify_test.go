package notify

import (
	"strings"
	"testing"
)

func TestEscapeAppleScriptNeutralisesQuotes(t *testing.T) {
	got := escapeAppleScript(`Marvel's "Spider-Man"`)
	if strings.Contains(got, `"Spider-Man"`) {
		t.Fatalf("double quotes were not escaped: %q", got)
	}
	if !strings.Contains(got, "Spider-Man") {
		t.Fatalf("escaping lost the title: %q", got)
	}
	if got := escapeAppleScript(`C:\games`); !strings.Contains(got, `\\`) {
		t.Fatalf("backslash was not escaped: %q", got)
	}
}

func TestEscapePowerShellDoublesSingleQuotes(t *testing.T) {
	got := escapePowerShell(`Marvel's`)
	if got != `Marvel''s` {
		t.Fatalf("got %q, want Marvel''s", got)
	}
}

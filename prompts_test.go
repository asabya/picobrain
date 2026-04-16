package picobrain

import (
	"os"
	"strings"
	"testing"
)

func TestObserverPromptNotEmpty(t *testing.T) {
	if ObserverPrompt == "" {
		t.Fatal("ObserverPrompt should not be empty")
	}
	if len(ObserverPrompt) < 100 {
		t.Errorf("ObserverPrompt too short: %d chars", len(ObserverPrompt))
	}
}

func TestReflectorPromptNotEmpty(t *testing.T) {
	if ReflectorPrompt == "" {
		t.Fatal("ReflectorPrompt should not be empty")
	}
	if len(ReflectorPrompt) < 100 {
		t.Errorf("ReflectorPrompt too short: %d chars", len(ReflectorPrompt))
	}
}

func TestPromptsMentionStructuredClaims(t *testing.T) {
	if !strings.Contains(ObserverPrompt, "summary") || !strings.Contains(ObserverPrompt, "claims") {
		t.Fatal("ObserverPrompt should mention summary and claims")
	}
	if !strings.Contains(ReflectorPrompt, "atomic claims") {
		t.Fatal("ReflectorPrompt should mention atomic claims")
	}
}

func TestObserverPromptNoLongerClaimsSpacyIsOptional(t *testing.T) {
	if strings.Contains(ObserverPrompt, "SpaCy is optional") {
		t.Fatalf("ObserverPrompt should not describe SpaCy as optional: %q", ObserverPrompt)
	}
	if strings.Contains(ObserverPrompt, "auto_graph") {
		t.Fatalf("ObserverPrompt should not mention auto_graph: %q", ObserverPrompt)
	}
}

func TestMandatorySpacyMigrationNotePresent(t *testing.T) {
	path := "docs/migrations/mandatory-spacy-startup.md"
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	text := string(content)
	for _, needle := range []string{"SpaCy", "mandatory", "EnableAutoGraph", "AutoInstallSpacy"} {
		if !strings.Contains(text, needle) {
			t.Fatalf("%s should mention %q", path, needle)
		}
	}
}

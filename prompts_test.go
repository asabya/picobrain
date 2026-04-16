package picobrain

import (
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

func TestObserverPromptRemovesLegacyOptionalSpacyGuidance(t *testing.T) {
	legacyPhrases := []string{
		"SpaCy is optional",
		"degrade gracefully",
		"auto_graph",
	}
	for _, phrase := range legacyPhrases {
		if strings.Contains(ObserverPrompt, phrase) {
			t.Fatalf("ObserverPrompt should not contain legacy SpaCy optionality phrase %q", phrase)
		}
	}
	if !strings.Contains(ObserverPrompt, "requires SpaCy") {
		t.Fatal("ObserverPrompt should describe mandatory SpaCy startup")
	}
}

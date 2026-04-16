package picobrain

import (
	"testing"
)

func TestSplitPathSeq(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{",abc123,", 1},
		{",abc123,def456,", 2},
		{",abc,def,ghi,", 3},
		{",,", 0},
		{"", 0},
	}

	for _, tt := range tests {
		got := splitPathSeq(tt.input)
		if len(got) != tt.want {
			t.Errorf("splitPathSeq(%q): got %d IDs, want %d", tt.input, len(got), tt.want)
		}
	}
}

func TestDepParserNotAvailable(t *testing.T) {
	brain := testBrain(t)

	// depParser should be nil in test brain (no spacy configured)
	if brain.depParser != nil {
		t.Error("expected depParser to be nil in test brain")
	}

	// ExtractTriples should return error
	_, err := brain.ExtractTriples(t.Context(), "test text")
	if err == nil {
		t.Error("expected error when depParser is nil")
	}
}

func TestResolveEntityEmpty(t *testing.T) {
	brain := testBrain(t)

	// resolveEntity with empty text should return empty
	result := brain.resolveEntity(t.Context(), "", 0.7)
	if result != "" {
		t.Errorf("expected empty result for empty entity, got %q", result)
	}
}
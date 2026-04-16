package picobrain

import (
	"reflect"
	"strings"
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

func TestExtractTriplesNoLongerMentionsAutoGraph(t *testing.T) {
	brain := &Brain{}

	_, err := brain.ExtractTriples(t.Context(), "test text")
	if err == nil {
		t.Fatal("expected error when depParser is nil")
	}
	if strings.Contains(err.Error(), "auto_graph") {
		t.Fatalf("expected error to stop mentioning auto_graph, got %q", err)
	}
}

func TestConfigRemovesOptionalSpacyFlags(t *testing.T) {
	cfgType := reflect.TypeOf(Config{})
	for _, fieldName := range []string{"EnableAutoGraph", "AutoInstallSpacy"} {
		if _, ok := cfgType.FieldByName(fieldName); ok {
			t.Fatalf("expected Config to remove %s", fieldName)
		}
	}
}

func TestConfigKeepsSpacyCacheDir(t *testing.T) {
	cfgType := reflect.TypeOf(Config{})
	if _, ok := cfgType.FieldByName("SpacyCacheDir"); !ok {
		t.Fatal("expected Config to keep SpacyCacheDir")
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

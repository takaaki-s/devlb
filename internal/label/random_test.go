package label

import (
	"strings"
	"testing"
)

func TestRandomLabel_Format(t *testing.T) {
	lbl := RandomLabel()
	parts := strings.SplitN(lbl, "-", 2)
	if len(parts) != 2 {
		t.Fatalf("expected 'adjective-noun' format, got %q", lbl)
	}
	if parts[0] == "" || parts[1] == "" {
		t.Fatalf("expected non-empty parts, got %q", lbl)
	}
}

func TestRandomLabel_Unique(t *testing.T) {
	seen := make(map[string]bool)
	n := 100
	for range n {
		seen[RandomLabel()] = true
	}
	// With ~20*20=400 combinations, 100 draws should yield at least 70 unique
	if len(seen) < 70 {
		t.Errorf("expected at least 70 unique labels from %d generations, got %d", n, len(seen))
	}
}

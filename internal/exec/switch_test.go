package exec

import (
	"testing"
)

func TestParseSwitchArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		listenPort int
		label      string
		wantErr    bool
	}{
		{"label only", []string{"feature-a"}, 0, "feature-a", false},
		{"port and label", []string{"3000", "feature-a"}, 3000, "feature-a", false},
		{"invalid port", []string{"abc", "feature-a"}, 0, "", true},
		{"no args", []string{}, 0, "", true},
		{"too many args", []string{"3000", "feature-a", "extra"}, 0, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lp, lbl, err := ParseSwitchArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseSwitchArgs(%v) expected error, got nil", tt.args)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseSwitchArgs(%v) error: %v", tt.args, err)
			}
			if lp != tt.listenPort {
				t.Errorf("listenPort = %d, want %d", lp, tt.listenPort)
			}
			if lbl != tt.label {
				t.Errorf("label = %q, want %q", lbl, tt.label)
			}
		})
	}
}

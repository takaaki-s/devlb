package exec

import (
	"reflect"
	"testing"
)

func TestParseExecPortArgs(t *testing.T) {
	tests := []struct {
		input        string
		listenPorts  []int
		backendPorts map[int]int
		wantErr      bool
	}{
		{"3000", []int{3000}, map[int]int{}, false},
		{"3000:3001", []int{3000}, map[int]int{3000: 3001}, false},
		{"8080:9090", []int{8080}, map[int]int{8080: 9090}, false},
		{"3000,8995", []int{3000, 8995}, map[int]int{}, false},
		{"3000:3001,8995:8996", []int{3000, 8995}, map[int]int{3000: 3001, 8995: 8996}, false},
		{"3000,8995:8996", []int{3000, 8995}, map[int]int{8995: 8996}, false},
		{"abc", nil, nil, true},
		{"3000:abc", nil, nil, true},
		{"abc:3000", nil, nil, true},
		{"", nil, nil, true},
		{"3000:3001:3002", nil, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			lp, bp, err := ParseExecPortArgs(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseExecPortArgs(%q) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseExecPortArgs(%q) error: %v", tt.input, err)
			}
			if !reflect.DeepEqual(lp, tt.listenPorts) {
				t.Errorf("listenPorts = %v, want %v", lp, tt.listenPorts)
			}
			if !reflect.DeepEqual(bp, tt.backendPorts) {
				t.Errorf("backendPorts = %v, want %v", bp, tt.backendPorts)
			}
		})
	}
}

package cmd

import (
	"encoding/json"
	"io"
	"os"
	"testing"
)

func TestIsJSON(t *testing.T) {
	tests := []struct {
		name   string
		format string
		want   bool
	}{
		{"json format", "json", true},
		{"text format", "text", false},
		{"empty format", "", false},
		{"unknown format", "xml", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			orig := outputFormat
			defer func() { outputFormat = orig }()
			outputFormat = tc.format
			if got := isJSON(); got != tc.want {
				t.Errorf("isJSON() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPrintJSON(t *testing.T) {
	tests := []struct {
		name  string
		input any
	}{
		{"string value", map[string]any{"key": "value"}},
		{"int value", map[string]any{"port": 3000}},
		{"bool value", map[string]any{"stopped": true}},
		{"nested map", map[string]any{"a": map[string]any{"b": 1}}},
		{"nil value", map[string]any{"ptr": nil}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			origStdout := os.Stdout
			os.Stdout = w
			defer func() { os.Stdout = origStdout }()

			if err := printJSON(tc.input); err != nil {
				t.Fatalf("printJSON() error: %v", err)
			}
			w.Close()

			out, err := io.ReadAll(r)
			if err != nil {
				t.Fatal(err)
			}

			if !json.Valid(out) {
				t.Errorf("printJSON() output is not valid JSON: %s", out)
			}

			var parsed map[string]any
			if err := json.Unmarshal(out, &parsed); err != nil {
				t.Errorf("printJSON() output cannot be unmarshalled: %v", err)
			}
		})
	}
}

func TestPrintJSONIndented(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStdout := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = origStdout }()

	input := map[string]any{"listen_port": 3000, "label": "test"}
	if err := printJSON(input); err != nil {
		t.Fatalf("printJSON() error: %v", err)
	}
	w.Close()

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}

	// Verify indentation is present (MarshalIndent with 2 spaces)
	expected, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent() error: %v", err)
	}
	// printJSON adds a trailing newline via fmt.Println
	expectedStr := string(expected) + "\n"
	if string(out) != expectedStr {
		t.Errorf("printJSON() output:\n%s\nwant:\n%s", out, expectedStr)
	}
}

func TestOutputFormatValidation(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		wantErr bool
	}{
		{"valid text", "text", false},
		{"valid json", "json", false},
		{"invalid xml", "xml", true},
		{"invalid empty", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			orig := outputFormat
			defer func() { outputFormat = orig }()
			outputFormat = tc.format

			err := rootCmd.PersistentPreRunE(rootCmd, nil)
			if (err != nil) != tc.wantErr {
				t.Errorf("PersistentPreRunE() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

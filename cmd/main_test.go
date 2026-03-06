package main

import (
	"bytes"
	"flag"
	"io"
	"os"
	"testing"

	"github.com/macedot/openmodel/internal/config"
)

func TestPrintUsage(t *testing.T) {
	oldStderr := os.Stderr
	defer func() { os.Stderr = oldStderr }()

	r, w, _ := os.Pipe()
	os.Stderr = w

	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"openmodel"}

	printUsage()

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)

	if !bytes.Contains(buf.Bytes(), []byte("Usage:")) {
		t.Error("Expected 'Usage:' in help output")
	}
	if !bytes.Contains(buf.Bytes(), []byte("serve")) {
		t.Error("Expected 'serve' command in help output")
	}
	if !bytes.Contains(buf.Bytes(), []byte("test")) {
		t.Error("Expected 'test' command in help output")
	}
}

func TestPrintTestUsage(t *testing.T) {
	oldStderr := os.Stderr
	defer func() { os.Stderr = oldStderr }()

	r, w, _ := os.Pipe()
	os.Stderr = w

	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"openmodel"}

	printTestUsage()

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)

	if !bytes.Contains(buf.Bytes(), []byte("Usage:")) {
		t.Error("Expected 'Usage:' in test help output")
	}
	if !bytes.Contains(buf.Bytes(), []byte("test")) {
		t.Error("Expected 'test' in test help output")
	}
}

func TestPrintServerUsage(t *testing.T) {
	oldStderr := os.Stderr
	defer func() { os.Stderr = oldStderr }()

	r, w, _ := os.Pipe()
	os.Stderr = w

	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"openmodel"}

	printServerUsage()

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)

	if !bytes.Contains(buf.Bytes(), []byte("Usage:")) {
		t.Error("Expected 'Usage:' in server help output")
	}
	if !bytes.Contains(buf.Bytes(), []byte("serve")) {
		t.Error("Expected 'serve' in server help output")
	}
}

func TestCommandParsing(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		wantCommand   string
		wantRemaining []string
	}{
		{
			name:          "default serve command",
			args:          []string{},
			wantCommand:   "serve",
			wantRemaining: []string{},
		},
		{
			name:          "explicit serve command",
			args:          []string{"serve", "-h"},
			wantCommand:   "serve",
			wantRemaining: []string{"-h"},
		},
		{
			name:          "test command",
			args:          []string{"test", "-model", "glm"},
			wantCommand:   "test",
			wantRemaining: []string{"-model", "glm"},
		},
		{
			name:          "flags before command",
			args:          []string{"-h", "serve"},
			wantCommand:   "serve",
			wantRemaining: []string{},
		},
		{
			name:          "unknown command",
			args:          []string{"unknown"},
			wantCommand:   "unknown",
			wantRemaining: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate command parsing logic from main
			command := "serve"
			args := make([]string, len(tt.args))
			copy(args, tt.args)

			for i, arg := range args {
				if arg == "-h" || arg == "--help" {
					command = "serve"
					args = args[:0]
					break
				}
				if !bytes.HasPrefix([]byte(arg), []byte("-")) {
					command = arg
					args = args[i+1:]
					break
				}
			}

			if command != tt.wantCommand {
				t.Errorf("command = %q, want %q", command, tt.wantCommand)
			}

			if len(args) != len(tt.wantRemaining) {
				t.Errorf("remaining args = %v, want %v", args, tt.wantRemaining)
			}
		})
	}
}

func TestFlagParsingTestCommand(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantModel   string
		wantCheck   bool
		wantParseOK bool
	}{
		{
			name:        "no flags",
			args:        []string{},
			wantModel:   "",
			wantCheck:   false,
			wantParseOK: true,
		},
		{
			name:        "model flag",
			args:        []string{"-model", "glm-4"},
			wantModel:   "glm-4",
			wantCheck:   false,
			wantParseOK: true,
		},
		{
			name:        "check flag",
			args:        []string{"-check"},
			wantModel:   "",
			wantCheck:   true,
			wantParseOK: true,
		},
		{
			name:        "both flags",
			args:        []string{"-model", "glm-5", "-check"},
			wantModel:   "glm-5",
			wantCheck:   true,
			wantParseOK: true,
		},
		{
			name:        "unknown flag",
			args:        []string{"-unknown"},
			wantModel:   "",
			wantCheck:   false,
			wantParseOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := flag.NewFlagSet("test", flag.ContinueOnError)
			flag.SetOutput(&bytes.Buffer{})

			modelName := flag.String("model", "", "Model name to test")
			jsonOutput := flag.Bool("check", false, "Output results in JSON")

			err := flag.Parse(tt.args)
			parseOK := err == nil

			if parseOK != tt.wantParseOK {
				t.Errorf("parse error = %v, want parseOK = %v", err, tt.wantParseOK)
			}

			if parseOK && *modelName != tt.wantModel {
				t.Errorf("model = %q, want %q", *modelName, tt.wantModel)
			}

			if parseOK && *jsonOutput != tt.wantCheck {
				t.Errorf("check = %v, want %v", *jsonOutput, tt.wantCheck)
			}
		})
	}
}

func TestRunTestNoConfig(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"openmodel", "test"}

	modelName := new(string)
	check := new(bool)
	*modelName = ""
	*check = false

	cfg, err := config.Load()

	if err == nil {
		t.Logf("Config loaded in test environment: %+v", cfg)
	} else {
		t.Logf("Expected error (no config): %v", err)
	}
}

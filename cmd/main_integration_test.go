package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/macedot/modelrouter/internal/config"
)

// These tests rely on a real config file in the developer's environment.
// They are deliberately skipped in CI unless the requisite file is present.

func TestRunModels_WithRealConfig(t *testing.T) {
	cfg, err := config.Load("")
	if err != nil {
		t.Skip("config file not found, skipping test")
	}
	configPath := cfg.GetConfigPath()
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skip("config file not found, skipping test")
	}

	oldConfig := config.FlagConfigPath
	defer func() { config.FlagConfigPath = oldConfig }()

	config.FlagConfigPath = configPath

	oldStdout := os.Stdout
	defer func() { os.Stdout = oldStdout }()

	r, w, _ := os.Pipe()
	os.Stdout = w

	runModels(false)

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "Available models:") {
		t.Fatalf("expected available models output, got: %s", output)
	}
}

func TestRunModels_JSONWithRealConfig(t *testing.T) {
	cfg, err := config.Load("")
	if err != nil {
		t.Skip("config file not found, skipping test")
	}
	configPath := cfg.GetConfigPath()
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skip("config file not found, skipping test")
	}

	oldConfig := config.FlagConfigPath
	defer func() { config.FlagConfigPath = oldConfig }()

	config.FlagConfigPath = configPath

	oldStdout := os.Stdout
	defer func() { os.Stdout = oldStdout }()

	r, w, _ := os.Pipe()
	os.Stdout = w

	runModels(true)

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "provider:") {
		t.Fatalf("expected provider info in JSON output, got: %s", output)
	}
}

func TestRunConfig_WithRealConfig(t *testing.T) {
	cfg, err := config.Load("")
	if err != nil {
		t.Skip("config file not found, skipping test")
	}
	configPath := cfg.GetConfigPath()
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skip("config file not found, skipping test")
	}

	oldConfig := config.FlagConfigPath
	defer func() { config.FlagConfigPath = oldConfig }()

	config.FlagConfigPath = configPath

	oldStdout := os.Stdout
	defer func() { os.Stdout = oldStdout }()

	r, w, _ := os.Pipe()
	os.Stdout = w

	runConfig()

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := strings.TrimSpace(buf.String())

	if !strings.HasSuffix(output, "ModelRouter.json") {
		t.Fatalf("expected config path ending with ModelRouter.json, got: %s", output)
	}
}

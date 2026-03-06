package logger

import (
	"testing"
)

func TestRedactSensitive_APIKey(t *testing.T) {
	input := `{"apiKey": "sk-12345", "name": "test"}`
	output := RedactSensitive(input)

	if contains(output, "sk-12345") {
		t.Errorf("apiKey was not redacted: %s", output)
	}
	if !contains(output, "[REDACTED]") {
		t.Errorf("expected [REDACTED] in output: %s", output)
	}
}

func TestRedactSensitive_MultipleFields(t *testing.T) {
	input := `{"apiKey": "sk-12345", "token": "abc123", "password": "secret123", "name": "test"}`
	output := RedactSensitive(input)

	if contains(output, "sk-12345") || contains(output, "abc123") || contains(output, "secret123") {
		t.Errorf("sensitive values not redacted: %s", output)
	}
	if !contains(output, `"name": "test"`) {
		t.Errorf("non-sensitive field was modified: %s", output)
	}
}

func TestRedactSensitive_CaseInsensitive(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"lowercase", `{"apikey": "secret"}`},
		{"uppercase", `{"APIKEY": "secret"}`},
		{"mixed", `{"ApiKey": "secret"}`},
		{"api_key", `{"API_KEY": "secret"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := RedactSensitive(tt.input)
			if contains(output, "secret") {
				t.Errorf("sensitive value not redacted: %s", output)
			}
		})
	}
}

func TestRedactSensitive_Empty(t *testing.T) {
	output := RedactSensitive("")
	if output != "" {
		t.Errorf("expected empty string, got: %s", output)
	}
}

func TestRedactSensitive_NoSensitiveFields(t *testing.T) {
	input := `{"name": "test", "value": 123}`
	output := RedactSensitive(input)
	if output != input {
		t.Errorf("non-sensitive JSON was modified: %s", output)
	}
}

func TestRedactHeaders_Authorization(t *testing.T) {
	headers := map[string]string{
		"Authorization": "Bearer sk-12345",
		"Content-Type":  "application/json",
	}

	redacted := RedactHeaders(headers)

	if redacted["Authorization"] != "[REDACTED]" {
		t.Errorf("Authorization header was not redacted: %s", redacted["Authorization"])
	}
	if redacted["Content-Type"] != "application/json" {
		t.Errorf("Content-Type header was modified: %s", redacted["Content-Type"])
	}
}

func TestRedactHeaders_Token(t *testing.T) {
	headers := map[string]string{
		"X-Access-Token": "token123",
		"X-Api-Key":      "key123",
		"X-Request-ID":   "req-abc",
	}

	redacted := RedactHeaders(headers)

	if redacted["X-Access-Token"] != "[REDACTED]" {
		t.Errorf("X-Access-Token was not redacted: %s", redacted["X-Access-Token"])
	}
	if redacted["X-Api-Key"] != "[REDACTED]" {
		t.Errorf("X-Api-Key was not redacted: %s", redacted["X-Api-Key"])
	}
	if redacted["X-Request-ID"] != "req-abc" {
		t.Errorf("X-Request-ID was modified: %s", redacted["X-Request-ID"])
	}
}

func TestRedactHeaders_Nil(t *testing.T) {
	redacted := RedactHeaders(nil)
	if redacted != nil {
		t.Errorf("expected nil, got: %v", redacted)
	}
}

func TestRedactURL_QueryParams(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string
	}{
		{
			name:     "api_key parameter",
			input:    "https://api.example.com/v1?api_key=secret123",
			contains: "api_key=[REDACTED]",
		},
		{
			name:     "token parameter",
			input:    "https://api.example.com/v1?token=abc123&other=value",
			contains: "token=[REDACTED]",
		},
		{
			name:     "multiple parameters",
			input:    "https://api.example.com/v1?api_key=secret&other=value",
			contains: "api_key=[REDACTED]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := RedactURL(tt.input)
			if !contains(output, tt.contains) {
				t.Errorf("expected %q in output: %s", tt.contains, output)
			}
			if contains(output, "secret123") || contains(output, "secret") || contains(output, "abc123") {
				t.Errorf("sensitive value in URL not redacted: %s", output)
			}
		})
	}
}

func TestRedactURL_Empty(t *testing.T) {
	output := RedactURL("")
	if output != "" {
		t.Errorf("expected empty string, got: %s", output)
	}
}

func TestRedactURL_NoSensitiveParams(t *testing.T) {
	input := "https://api.example.com/v1?model=gpt-4"
	output := RedactURL(input)
	if output != input {
		t.Errorf("URL without sensitive params was modified: %s", output)
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

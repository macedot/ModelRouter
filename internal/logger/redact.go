// Package logger provides structured logging for openmodel.
package logger

import (
	"regexp"
	"strings"
)

// sensitiveFields contains field names that should be redacted in logs
var sensitiveFields = []string{
	"apiKey",
	"api_key",
	"key",
	"token",
	"password",
	"secret",
	"authorization",
	"credential",
	"private",
	"access_token",
	"refresh_token",
	"bearer",
}

// compiledPatterns caches compiled regex patterns for redaction
var compiledPatterns []*regexp.Regexp

func init() {
	// Pre-compile patterns for performance
	for _, field := range sensitiveFields {
		// Match "field": "value" patterns in JSON (case-insensitive)
		pattern := `(?i)"` + regexp.QuoteMeta(field) + `"\s*:\s*"[^"]*"`
		compiledPatterns = append(compiledPatterns, regexp.MustCompile(pattern))
	}
}

// RedactSensitive replaces sensitive field values with [REDACTED] in JSON strings.
// This prevents credentials from being logged in plain text.
func RedactSensitive(data string) string {
	if data == "" {
		return data
	}

	for _, re := range compiledPatterns {
		matches := re.FindAllString(data, -1)
		for _, match := range matches {
			// Replace the value part with [REDACTED]
			redacted := regexp.MustCompile(`:\s*"[^"]*"`).ReplaceAllString(match, `: "[REDACTED]"`)
			data = strings.Replace(data, match, redacted, 1)
		}
	}
	return data
}

// RedactHeaders redacts sensitive HTTP header values.
// This prevents Authorization headers from being logged in plain text.
func RedactHeaders(headers map[string]string) map[string]string {
	if headers == nil {
		return nil
	}

	redacted := make(map[string]string, len(headers))
	for k, v := range headers {
		lowerKey := strings.ToLower(k)
		if lowerKey == "authorization" ||
			lowerKey == "api-key" ||
			strings.Contains(lowerKey, "api-key") ||
			strings.Contains(lowerKey, "token") ||
			strings.Contains(lowerKey, "secret") ||
			strings.Contains(lowerKey, "password") ||
			strings.Contains(lowerKey, "key") {
			redacted[k] = "[REDACTED]"
		} else {
			redacted[k] = v
		}
	}
	return redacted
}

// RedactURL redacts sensitive query parameters from URLs.
// This prevents API keys in URL parameters from being logged.
func RedactURL(url string) string {
	if url == "" {
		return url
	}

	// Redact common sensitive query parameters
	sensitiveParams := []string{"api_key", "apikey", "key", "token", "secret", "password"}
	for _, param := range sensitiveParams {
		// Match param=value pattern (case-insensitive)
		pattern := regexp.MustCompile(`(?i)(` + regexp.QuoteMeta(param) + `=)[^&]*`)
		url = pattern.ReplaceAllString(url, `$1[REDACTED]`)
	}
	return url
}

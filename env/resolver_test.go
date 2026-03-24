package env

import (
	"os"
	"testing"
)

func TestResolve(t *testing.T) {
	// Set up test environment variables
	os.Setenv("TEST_VAR", "test_value")
	os.Setenv("TELEGRAM_TOKEN", "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11")
	os.Setenv("DISCORD_WEBHOOK", "https://discord.com/api/webhooks/123/abc")
	defer func() {
		os.Unsetenv("TEST_VAR")
		os.Unsetenv("TELEGRAM_TOKEN")
		os.Unsetenv("DISCORD_WEBHOOK")
	}()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "no pattern",
			input:    "plain text",
			expected: "plain text",
		},
		{
			name:     "single variable",
			input:    "${TEST_VAR}",
			expected: "test_value",
		},
		{
			name:     "variable with prefix",
			input:    "prefix_${TEST_VAR}",
			expected: "prefix_test_value",
		},
		{
			name:     "variable with suffix",
			input:    "${TEST_VAR}_suffix",
			expected: "test_value_suffix",
		},
		{
			name:     "variable with prefix and suffix",
			input:    "prefix_${TEST_VAR}_suffix",
			expected: "prefix_test_value_suffix",
		},
		{
			name:     "multiple variables",
			input:    "${TEST_VAR}_${TELEGRAM_TOKEN}",
			expected: "test_value_123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		},
		{
			name:     "unset variable",
			input:    "${UNSET_VAR}",
			expected: "",
		},
		{
			name:     "mixed set and unset",
			input:    "${TEST_VAR}_${UNSET_VAR}_${TELEGRAM_TOKEN}",
			expected: "test_value__123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		},
		{
			name:     "telegram token pattern",
			input:    "${TELEGRAM_TOKEN}",
			expected: "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		},
		{
			name:     "discord webhook pattern",
			input:    "${DISCORD_WEBHOOK}",
			expected: "https://discord.com/api/webhooks/123/abc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Resolve(tt.input)
			if result != tt.expected {
				t.Errorf("Resolve(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestResolveRequired(t *testing.T) {
	// Set up test environment variables
	os.Setenv("TEST_VAR", "test_value")
	os.Setenv("REQUIRED_TOKEN", "secret_token")
	defer func() {
		os.Unsetenv("TEST_VAR")
		os.Unsetenv("REQUIRED_TOKEN")
	}()

	tests := []struct {
		name        string
		input       string
		expected    string
		expectError bool
	}{
		{
			name:        "empty string",
			input:       "",
			expected:    "",
			expectError: false,
		},
		{
			name:        "set variable",
			input:       "${TEST_VAR}",
			expected:    "test_value",
			expectError: false,
		},
		{
			name:        "unset variable",
			input:       "${UNSET_VAR}",
			expected:    "",
			expectError: true,
		},
		{
			name:        "multiple variables all set",
			input:       "${TEST_VAR}_${REQUIRED_TOKEN}",
			expected:    "test_value_secret_token",
			expectError: false,
		},
		{
			name:        "multiple variables one unset",
			input:       "${TEST_VAR}_${UNSET_VAR}",
			expected:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ResolveRequired(tt.input)
			if tt.expectError {
				if err == nil {
					t.Errorf("ResolveRequired(%q) expected error, got nil", tt.input)
				}
			} else {
				if err != nil {
					t.Errorf("ResolveRequired(%q) unexpected error: %v", tt.input, err)
				}
				if result != tt.expected {
					t.Errorf("ResolveRequired(%q) = %q, want %q", tt.input, result, tt.expected)
				}
			}
		})
	}
}

func TestMissingVarError(t *testing.T) {
	tests := []struct {
		name     string
		err      *MissingVarError
		expected string
	}{
		{
			name:     "single variable",
			err:      &MissingVarError{Variables: []string{"VAR1"}},
			expected: "required environment variable not set: VAR1",
		},
		{
			name:     "multiple variables",
			err:      &MissingVarError{Variables: []string{"VAR1", "VAR2", "VAR3"}},
			expected: "required environment variables not set: VAR1, VAR2, VAR3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.Error()
			if result != tt.expected {
				t.Errorf("MissingVarError.Error() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestHasEnvPattern(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "no pattern",
			input:    "plain text",
			expected: false,
		},
		{
			name:     "single pattern",
			input:    "${VAR}",
			expected: true,
		},
		{
			name:     "pattern with text",
			input:    "prefix_${VAR}_suffix",
			expected: true,
		},
		{
			name:     "multiple patterns",
			input:    "${VAR1}_${VAR2}",
			expected: true,
		},
		{
			name:     "incomplete pattern",
			input:    "$VAR",
			expected: false,
		},
		{
			name:     "incomplete pattern with brace",
			input:    "${VAR",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasEnvPattern(tt.input)
			if result != tt.expected {
				t.Errorf("HasEnvPattern(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestResolveInvalidVariableName(t *testing.T) {
	// Test that invalid variable names are not resolved
	os.Setenv("VALID_VAR", "value")
	defer os.Unsetenv("VALID_VAR")

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "invalid variable name with dash",
			input:    "${INVALID-VAR}",
			expected: "${INVALID-VAR}", // Not matched, returned as is
		},
		{
			name:     "invalid variable name starting with number",
			input:    "${1INVALID}",
			expected: "${1INVALID}", // Not matched, returned as is
		},
		{
			name:     "valid variable name",
			input:    "${VALID_VAR}",
			expected: "value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Resolve(tt.input)
			if result != tt.expected {
				t.Errorf("Resolve(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

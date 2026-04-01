package main

import (
	"testing"
)

func TestValidateTokenStrength(t *testing.T) {
	tests := []struct {
		name     string
		token    string
		expected int
	}{
		{
			name:     "very weak - too short",
			token:    "abc123",
			expected: 1,
		},
		{
			name:     "very weak - short single type",
			token:    "abcdefg",
			expected: 1,
		},
		{
			name:     "weak - 8 chars single type",
			token:    "abcdefgh",
			expected: 2,
		},
		{
			name:     "weak - 10 chars digits only",
			token:    "1234567890",
			expected: 2,
		},
		{
			name:     "moderate - 12 chars",
			token:    "abcdefgh1234",
			expected: 3,
		},
		{
			name:     "moderate - 2 char types",
			token:    "abcd1234",
			expected: 3,
		},
		{
			name:     "strong - 16 chars 3 types",
			token:    "abcd1234EFGH5678",
			expected: 4,
		},
		{
			name:     "very strong - 16 chars 4 types",
			token:    "aB3$xY9@mN2&kL7!",
			expected: 5,
		},
		{
			name:     "very strong - long with special chars",
			token:    "MyStr0ng_P@ssw0rd!",
			expected: 5,
		},
		{
			name:     "edge case - exactly 8 chars lowercase",
			token:    "abcd1234",
			expected: 3,
		},
		{
			name:     "edge case - 15 chars 3 types",
			token:    "Abc123Def456Ghi",
			expected: 3,
		},
		{
			name:     "edge case - 16 chars 3 types no special",
			token:    "Abcdefgh12345678",
			expected: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateTokenStrength(tt.token)
			if got != tt.expected {
				t.Errorf("validateTokenStrength(%q) = %d, want %d", tt.token, got, tt.expected)
			}
		})
	}
}

func TestValidateTokenStrength_SpecialCharacters(t *testing.T) {
	specialChars := []string{"!", "@", "#", "$", "%", "^", "&", "*", "(", ")", "-", "_", "=", "+", "[", "]", "{", "}", "|", "\\", ";", ":", "'", "\"", ",", ".", "<", ">", "/", "?", "`", "~"}

	for _, special := range specialChars {
		token := "Abcd1234" + special + "Xy"
		got := validateTokenStrength(token)
		if got < 4 {
			t.Errorf("Token with special char %q should be strong, got score %d", special, got)
		}
	}
}

func TestValidateTokenStrength_EmptyAndShort(t *testing.T) {
	tests := []struct {
		token    string
		expected int
	}{
		{"", 1},
		{"a", 1},
		{"ab", 1},
		{"abc", 1},
		{"abcd", 1},
		{"abcde", 1},
		{"abcdef", 1},
		{"abcdefg", 1},
	}

	for _, tt := range tests {
		got := validateTokenStrength(tt.token)
		if got != tt.expected {
			t.Errorf("validateTokenStrength(%q) = %d, want %d", tt.token, got, tt.expected)
		}
	}
}

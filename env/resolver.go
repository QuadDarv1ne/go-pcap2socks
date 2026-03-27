// Package env provides environment variable resolution for configuration values.
// It supports ${VAR_NAME} syntax for embedding environment variables.
package env

import (
	"errors"
	"os"
	"regexp"
	"strings"
)

// Pre-defined errors for environment variable resolution
var (
	ErrMissingVar = errors.New("required environment variable not set")
)

// envPattern matches ${VAR_NAME} patterns
var envPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// Resolve replaces ${VAR_NAME} patterns with environment variable values.
// If a variable is not set, it returns an empty string for that pattern.
//
// Examples:
//   - "${TELEGRAM_TOKEN}" → "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11"
//   - "prefix_${VAR}_suffix" → "prefix_value_suffix"
//   - "${UNSET_VAR}" → ""
func Resolve(value string) string {
	if value == "" {
		return ""
	}

	return envPattern.ReplaceAllStringFunc(value, func(match string) string {
		// Extract variable name (remove ${ and })
		varName := strings.TrimSuffix(strings.TrimPrefix(match, "${"), "}")
		return os.Getenv(varName)
	})
}

// ResolveRequired replaces ${VAR_NAME} patterns with environment variable values.
// If a variable is not set, it returns an error.
//
// This is useful for required configuration values like API tokens.
func ResolveRequired(value string) (string, error) {
	if value == "" {
		return "", nil
	}

	var missingVars []string

	result := envPattern.ReplaceAllStringFunc(value, func(match string) string {
		varName := strings.TrimSuffix(strings.TrimPrefix(match, "${"), "}")
		val := os.Getenv(varName)
		if val == "" {
			missingVars = append(missingVars, varName)
		}
		return val
	})

	if len(missingVars) > 0 {
		return "", &MissingVarError{Variables: missingVars}
	}

	return result, nil
}

// MissingVarError is returned when a required environment variable is not set
type MissingVarError struct {
	Variables []string
}

func (e *MissingVarError) Error() string {
	if len(e.Variables) == 1 {
		return ErrMissingVar.Error() + ": " + e.Variables[0]
	}
	return ErrMissingVar.Error() + "s: " + strings.Join(e.Variables, ", ")
}

// HasEnvPattern checks if a string contains ${VAR_NAME} patterns
func HasEnvPattern(value string) bool {
	if value == "" {
		return false
	}
	return envPattern.MatchString(value)
}

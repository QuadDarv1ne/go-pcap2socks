package cfg

import (
	"fmt"
	"strconv"
	"strings"
)

// PortRange represents a range of ports [start, end] inclusive
type PortRange struct {
	Start uint16
	End   uint16
}

// Contains checks if a port is within this range
func (pr PortRange) Contains(port uint16) bool {
	return port >= pr.Start && port <= pr.End
}

// PortMatcher efficiently matches ports using ranges instead of maps
type PortMatcher struct {
	ranges []PortRange
}

// NewPortMatcher creates a new port matcher from a port specification string
// Format: "80,443,8000-9000"
func NewPortMatcher(spec string) (*PortMatcher, error) {
	if spec == "" {
		return &PortMatcher{ranges: nil}, nil
	}

	var ranges []PortRange

	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)

		if strings.Contains(part, "-") {
			// Parse range
			parts := strings.Split(part, "-")
			if len(parts) != 2 {
				return nil, fmt.Errorf("invalid port range format: %s", part)
			}

			start, err := strconv.ParseUint(strings.TrimSpace(parts[0]), 10, 16)
			if err != nil {
				return nil, fmt.Errorf("invalid port range start %s: %w", parts[0], err)
			}

			end, err := strconv.ParseUint(strings.TrimSpace(parts[1]), 10, 16)
			if err != nil {
				return nil, fmt.Errorf("invalid port range end %s: %w", parts[1], err)
			}

			if start > end {
				return nil, fmt.Errorf("invalid port range %s: start > end", part)
			}

			ranges = append(ranges, PortRange{Start: uint16(start), End: uint16(end)})
		} else {
			// Parse single port
			port, err := strconv.ParseUint(part, 10, 16)
			if err != nil {
				return nil, fmt.Errorf("invalid port %s: %w", part, err)
			}

			ranges = append(ranges, PortRange{Start: uint16(port), End: uint16(port)})
		}
	}

	return &PortMatcher{ranges: ranges}, nil
}

// Matches checks if a port matches any of the ranges
func (pm *PortMatcher) Matches(port uint16) bool {
	if pm == nil || len(pm.ranges) == 0 {
		return false
	}

	for i := range pm.ranges {
		if pm.ranges[i].Contains(port) {
			return true
		}
	}

	return false
}

// IsEmpty returns true if the matcher has no ranges
func (pm *PortMatcher) IsEmpty() bool {
	return pm == nil || len(pm.ranges) == 0
}

// ParsePortRange parses a port range string and returns error if invalid
// Format: "80,443,8000-9000"
func ParsePortRange(spec string) error {
	if spec == "" {
		return nil
	}

	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)

		if strings.Contains(part, "-") {
			// Parse range
			parts := strings.Split(part, "-")
			if len(parts) != 2 {
				return fmt.Errorf("invalid port range format: %s", part)
			}

			start, err := strconv.ParseUint(strings.TrimSpace(parts[0]), 10, 16)
			if err != nil {
				return fmt.Errorf("invalid port range start %s: %w", parts[0], err)
			}

			end, err := strconv.ParseUint(strings.TrimSpace(parts[1]), 10, 16)
			if err != nil {
				return fmt.Errorf("invalid port range end %s: %w", parts[1], err)
			}

			if start > end {
				return fmt.Errorf("invalid port range %s: start > end", part)
			}

			if start == 0 || end > 65535 {
				return fmt.Errorf("port must be 1-65535, got %d-%d", start, end)
			}
		} else {
			// Parse single port
			port, err := strconv.ParseUint(part, 10, 16)
			if err != nil {
				return fmt.Errorf("invalid port %s: %w", part, err)
			}

			if port == 0 || port > 65535 {
				return fmt.Errorf("port must be 1-65535, got %d", port)
			}
		}
	}

	return nil
}

package stats

import (
	"time"
)

// cleanupLoop periodically removes inactive devices
func (s *Store) cleanupLoop() {
	defer s.cleanupWg.Done()

	ticker := time.NewTicker(s.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.CleanupInactive()
		case <-s.stopCleanup:
			return
		}
	}
}

// CleanupInactive removes devices that haven't been seen for longer than inactivityTimeout
func (s *Store) CleanupInactive() int {
	if s.inactivityTimeout == 0 {
		return 0
	}

	cutoff := time.Now().Add(-s.inactivityTimeout)

	s.mu.Lock()
	defer s.mu.Unlock()

	removed := 0
	for ip, device := range s.devices {
		// Load LastSeen atomically if possible, otherwise use mutex
		lastSeen := device.LastSeen
		if lastSeen.Before(cutoff) {
			delete(s.devices, ip)
			removed++
		}
	}

	return removed
}

// Stop stops the cleanup goroutine
func (s *Store) Stop() {
	if s.stopCleanup != nil {
		close(s.stopCleanup)
		s.cleanupWg.Wait()
	}
}

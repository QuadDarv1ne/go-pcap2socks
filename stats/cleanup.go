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

	now := time.Now()
	cutoff := now.Add(-s.inactivityTimeout)

	s.mu.Lock()
	defer s.mu.Unlock()

	removed := 0
	for ip, device := range s.devices {
		device.mu.RLock()
		lastSeen := device.LastSeen
		device.mu.RUnlock()

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

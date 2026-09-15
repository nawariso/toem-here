package identity

import "time"

// ExpireCacheForTest invalidates the cached signing keys and the refresh
// cooldown so tests can exercise key rotation without waiting in real time.
func ExpireCacheForTest(v *Verifier) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.expires = time.Time{}
	v.lastAttempt = time.Time{}
}

package services

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrRateLimited is returned when a call is rejected locally — either by our
// own per-minute budget for the shared Cohere trial key, or because a prior
// 429 from Cohere put us in a cooldown window. Either way we fail fast
// without making the network call, so one visitor spamming requests can't
// burn through the shared quota for everyone else.
var ErrRateLimited = errors.New("AI service is busy right now, please wait a moment and try again")

// fixedWindowLimiter is a minimal per-minute request budget. A fixed window
// is good enough here — the limits are generous (5-10/min) and this is a
// single-instance process, so a hand-rolled counter avoids pulling in a
// token-bucket dependency for something this small.
type fixedWindowLimiter struct {
	mu          sync.Mutex
	max         int
	windowStart time.Time
	count       int
}

func newFixedWindowLimiter(maxPerMinute int) *fixedWindowLimiter {
	return &fixedWindowLimiter{max: maxPerMinute, windowStart: time.Now()}
}

func (l *fixedWindowLimiter) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	if now.Sub(l.windowStart) >= time.Minute {
		l.windowStart = now
		l.count = 0
	}
	if l.count >= l.max {
		return false
	}
	l.count++
	return true
}

var (
	embedLimiter *fixedWindowLimiter
	chatLimiter  *fixedWindowLimiter

	cohereCooldownMu    sync.Mutex
	cohereCooldownUntil time.Time
)

const cohereCooldown = 60 * time.Second

// InitRateLimits configures the shared per-minute budgets for Cohere calls.
// Call once at startup, after InitOpenAI.
func InitRateLimits(embedRPM, chatRPM int) {
	embedLimiter = newFixedWindowLimiter(embedRPM)
	chatLimiter = newFixedWindowLimiter(chatRPM)
}

// cohereCoolingDown reports whether we're inside a post-429 circuit-breaker
// cooldown, so callers can fail fast instead of hammering an already
// rate-limited key.
func cohereCoolingDown() bool {
	cohereCooldownMu.Lock()
	defer cohereCooldownMu.Unlock()
	return time.Now().Before(cohereCooldownUntil)
}

// tripCohereCooldown opens the circuit breaker after Cohere itself returns
// a 429, so subsequent calls back off for a while instead of retrying
// immediately into the same throttle.
func tripCohereCooldown() {
	cohereCooldownMu.Lock()
	defer cohereCooldownMu.Unlock()
	cohereCooldownUntil = time.Now().Add(cohereCooldown)
}

// checkBudget fails fast (no network call) if the circuit breaker is open
// or the local per-minute budget for this call type is exhausted.
func checkBudget(limiter *fixedWindowLimiter) error {
	if cohereCoolingDown() {
		return ErrRateLimited
	}
	if limiter != nil && !limiter.Allow() {
		return ErrRateLimited
	}
	return nil
}

// isRateLimitStatus reports whether a Cohere HTTP status indicates the
// shared key itself got throttled (as opposed to a client error).
func isRateLimitStatus(status int) bool {
	return status == 429
}

func cohereStatusError(status int, body string) error {
	if isRateLimitStatus(status) {
		tripCohereCooldown()
	}
	return fmt.Errorf("Cohere API error (status %d): %s", status, body)
}

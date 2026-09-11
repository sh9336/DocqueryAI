// Package session implements an in-memory, mutex-guarded session lease for a
// single-instance deployment (no Postgres table needed — the process itself
// is the source of truth, which naturally resets on redeploy/restart).
package session

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"sync"
	"time"
)

var (
	ErrDemoFull      = errors.New("demo is at capacity")
	ErrIPLimit       = errors.New("only one active session per visitor is allowed")
	ErrNotFound      = errors.New("session not found or expired")
	ErrBusy          = errors.New("a request is already in progress for this session")
	ErrDocLimit      = errors.New("document limit reached for this session")
	ErrChunkLimit    = errors.New("chunk limit reached for this session")
	ErrQuestionLimit = errors.New("question limit reached for this session")
)

type Limits struct {
	MaxSessions      int
	MaxSessionsPerIP int
	HeartbeatTimeout time.Duration
	MaxLifetime      time.Duration
	MaxDocs          int
	MaxChunks        int
	MaxQuestions     int
}

// Session holds per-visitor lease state and usage counters. Copies (via
// Snapshot) are safe to read without the manager's lock; the live pointer
// must only be touched while holding Manager.mu.
type Session struct {
	ID            string
	IP            string
	CreatedAt     time.Time
	LastSeen      time.Time
	DocCount      int
	ChunkCount    int
	QuestionCount int
	uploadBusy    bool
	chatBusy      bool
}

type Manager struct {
	mu       sync.Mutex
	sessions map[string]*Session
	byIP     map[string]int
	limits   Limits
	cleanup  func(sessionID string)
}

// NewManager creates a session manager. cleanup is invoked (in its own
// goroutine) with a session's ID whenever that session is removed, whether
// by explicit release or by expiry — wire it to delete the session's DB rows
// and uploaded files.
func NewManager(limits Limits, cleanup func(sessionID string)) *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
		byIP:     make(map[string]int),
		limits:   limits,
		cleanup:  cleanup,
	}
}

func newSessionID() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic("session: failed to generate random ID: " + err.Error())
	}
	return hex.EncodeToString(b)
}

func (m *Manager) expired(s *Session, now time.Time) bool {
	return now.Sub(s.LastSeen) > m.limits.HeartbeatTimeout || now.Sub(s.CreatedAt) > m.limits.MaxLifetime
}

// sweepLocked removes expired sessions. Caller must hold m.mu.
func (m *Manager) sweepLocked() {
	now := time.Now()
	for id, s := range m.sessions {
		if m.expired(s, now) {
			m.removeLocked(id, s)
		}
	}
}

// removeLocked deletes a session and schedules its cleanup. Caller must hold m.mu.
func (m *Manager) removeLocked(id string, s *Session) {
	delete(m.sessions, id)
	m.byIP[s.IP]--
	if m.byIP[s.IP] <= 0 {
		delete(m.byIP, s.IP)
	}
	if m.cleanup != nil {
		go m.cleanup(id)
	}
}

// Create acquires a new session lease for ip, enforcing the global and
// per-IP concurrency caps. It sweeps expired sessions first so a slot freed
// by inactivity is available immediately rather than waiting on the
// background sweeper.
func (m *Manager) Create(ip string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.sweepLocked()

	if len(m.sessions) >= m.limits.MaxSessions {
		return nil, ErrDemoFull
	}
	if m.byIP[ip] >= m.limits.MaxSessionsPerIP {
		return nil, ErrIPLimit
	}

	now := time.Now()
	s := &Session{
		ID:        newSessionID(),
		IP:        ip,
		CreatedAt: now,
		LastSeen:  now,
	}
	m.sessions[s.ID] = s
	m.byIP[ip]++
	log.Printf("[SESSION] Created %s for %s (%d/%d active)", s.ID, ip, len(m.sessions), m.limits.MaxSessions)
	return s, nil
}

// Get looks up a session, lazily expiring it if stale.
func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.sessions[id]
	if !ok {
		return nil, false
	}
	if m.expired(s, time.Now()) {
		m.removeLocked(id, s)
		return nil, false
	}
	return s, true
}

// Touch refreshes LastSeen, extending the session's inactivity window.
func (m *Manager) Touch(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.sessions[id]
	if !ok {
		return false
	}
	if m.expired(s, time.Now()) {
		m.removeLocked(id, s)
		return false
	}
	s.LastSeen = time.Now()
	return true
}

// Release immediately ends a session (explicit logout / tab-close beacon),
// freeing its slot without waiting for the inactivity timeout.
func (m *Manager) Release(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.sessions[id]
	if !ok {
		return
	}
	m.removeLocked(id, s)
	log.Printf("[SESSION] Released %s (%d/%d active)", id, len(m.sessions), m.limits.MaxSessions)
}

// StartSweeper runs a background goroutine that periodically expires stale
// sessions even when no new requests arrive to trigger a lazy check — so
// abandoned sessions' data gets cleaned up promptly.
func (m *Manager) StartSweeper(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			m.mu.Lock()
			m.sweepLocked()
			m.mu.Unlock()
		}
	}()
}

// --- Per-session concurrency guards (max 1 in-flight upload/chat each) ---

func (m *Manager) TryStartUpload(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok || m.expired(s, time.Now()) {
		return ErrNotFound
	}
	if s.uploadBusy {
		return ErrBusy
	}
	s.uploadBusy = true
	return nil
}

func (m *Manager) FinishUpload(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[id]; ok {
		s.uploadBusy = false
	}
}

func (m *Manager) TryStartChat(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok || m.expired(s, time.Now()) {
		return ErrNotFound
	}
	if s.chatBusy {
		return ErrBusy
	}
	s.chatBusy = true
	return nil
}

func (m *Manager) FinishChat(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[id]; ok {
		s.chatBusy = false
	}
}

// --- Per-session resource counters ---

// CheckAndIncrDocs atomically validates and applies a new document's usage
// against the doc/chunk caps, so concurrent requests can't both pass the
// check before either increments (TOCTOU-safe).
func (m *Manager) CheckAndIncrDocs(id string, chunks int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return ErrNotFound
	}
	if s.DocCount >= m.limits.MaxDocs {
		return ErrDocLimit
	}
	if s.ChunkCount+chunks > m.limits.MaxChunks {
		return ErrChunkLimit
	}
	s.DocCount++
	s.ChunkCount += chunks
	return nil
}

// DecrDoc reverses CheckAndIncrDocs when a document is deleted.
func (m *Manager) DecrDoc(id string, chunks int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return
	}
	s.DocCount--
	if s.DocCount < 0 {
		s.DocCount = 0
	}
	s.ChunkCount -= chunks
	if s.ChunkCount < 0 {
		s.ChunkCount = 0
	}
}

// CheckAndIncrQuestions atomically validates and applies the per-session
// question quota.
func (m *Manager) CheckAndIncrQuestions(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return ErrNotFound
	}
	if s.QuestionCount >= m.limits.MaxQuestions {
		return ErrQuestionLimit
	}
	s.QuestionCount++
	return nil
}

// Snapshot returns a value copy of the session's current counters, safe to
// read/serialize without holding the manager's lock.
func (m *Manager) Snapshot(id string) (Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return Session{}, false
	}
	return *s, true
}

// Limits exposes the configured limits (e.g. for the /session response body).
func (m *Manager) Limits() Limits {
	return m.limits
}

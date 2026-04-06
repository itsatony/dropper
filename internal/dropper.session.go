package dropper

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Session represents an authenticated user session.
type Session struct {
	Token        string
	CreatedAt    time.Time
	ExpiresAt    time.Time
	LastAccessed time.Time
}

// SessionStore manages in-memory sessions with TTL and periodic cleanup.
type SessionStore struct {
	mu       sync.Mutex // NOT RWMutex — Get always updates LastAccessed
	sessions map[string]*Session
	ttl      time.Duration
	logger   *slog.Logger
	stopCh   chan struct{}
	stopOnce sync.Once
}

// NewSessionStore creates a new SessionStore with the given TTL and logger,
// and starts a background cleanup goroutine.
func NewSessionStore(ttl time.Duration, logger *slog.Logger) *SessionStore {
	s := &SessionStore{
		sessions: make(map[string]*Session),
		ttl:      ttl,
		logger:   logger,
		stopCh:   make(chan struct{}),
	}

	logger.Info(LogMsgSessionStoreStart)

	go s.cleanupLoop()

	return s
}

// cleanupLoop runs in the background, periodically removing expired sessions.
func (s *SessionStore) cleanupLoop() {
	ticker := time.NewTicker(SessionCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			n := s.CleanupExpired()
			s.logger.Debug(LogMsgSessionCleanup, slog.Int(LogFieldExpired, n))
		case <-s.stopCh:
			s.logger.Info(LogMsgSessionStoreStop)
			return
		}
	}
}

// Create generates a new session with a crypto/rand token, stores it, and
// returns the token. An error is returned only if crypto/rand fails.
func (s *SessionStore) Create() (string, error) {
	token, err := generateToken()
	if err != nil {
		return "", fmt.Errorf("%s: %w", ErrMsgTokenGeneration, err)
	}

	now := time.Now()
	session := &Session{
		Token:        token,
		CreatedAt:    now,
		ExpiresAt:    now.Add(s.ttl),
		LastAccessed: now,
	}

	s.mu.Lock()
	s.sessions[token] = session
	s.mu.Unlock()

	s.logger.Info(LogMsgSessionCreated,
		slog.String(LogFieldSessionID, sessionTokenPrefix(token)),
	)

	return token, nil
}

// Get returns the session for the given token, or nil if not found or expired.
// On a hit it updates LastAccessed. Expired sessions are deleted on access.
// Takes a write lock because it always mutates LastAccessed on success.
func (s *SessionStore) Get(token string) *Session {
	s.mu.Lock()

	session, ok := s.sessions[token]
	if !ok {
		s.mu.Unlock()
		return nil
	}

	if time.Now().After(session.ExpiresAt) {
		delete(s.sessions, token)
		s.mu.Unlock()
		s.logger.Debug(LogMsgSessionExpired,
			slog.String(LogFieldSessionID, sessionTokenPrefix(token)),
		)
		return nil
	}

	session.LastAccessed = time.Now()
	s.mu.Unlock()
	return session
}

// Delete removes the session identified by token. It is a no-op if the token
// is not found.
func (s *SessionStore) Delete(token string) {
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}

// CleanupExpired removes all expired sessions from the store and returns the
// number of sessions removed.
func (s *SessionStore) CleanupExpired() int {
	now := time.Now()
	removed := 0

	s.mu.Lock()
	for token, session := range s.sessions {
		if now.After(session.ExpiresAt) {
			delete(s.sessions, token)
			removed++
		}
	}
	s.mu.Unlock()

	return removed
}

// Stop signals the background cleanup goroutine to exit. It is safe to call
// multiple times; subsequent calls are no-ops.
func (s *SessionStore) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
}

// generateToken returns a hex-encoded string of SessionTokenBytes random bytes
// produced by crypto/rand.
func generateToken() (string, error) {
	buf := make([]byte, SessionTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// sessionTokenPrefix returns the first SessionTokenLogPrefixLen characters of
// the token for safe log output. If the token is shorter than that, the full
// token is returned.
func sessionTokenPrefix(token string) string {
	if len(token) <= SessionTokenLogPrefixLen {
		return token
	}
	return token[:SessionTokenLogPrefixLen]
}

package dropper

import (
	"io"
	"log/slog"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testSessionStore creates a SessionStore with the given TTL, discards all
// log output, and registers Stop as a test-cleanup function.
func testSessionStore(t *testing.T, ttl time.Duration) *SessionStore {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := NewSessionStore(ttl, logger)
	t.Cleanup(func() { store.Stop() })
	return store
}

func TestSessionStore_Create(t *testing.T) {
	store := testSessionStore(t, time.Hour)

	token, err := store.Create()
	require.NoError(t, err)
	require.NotEmpty(t, token)

	// Token must be exactly 64 hex characters (SessionTokenBytes=32 -> 64 hex chars).
	assert.Len(t, token, SessionTokenBytes*2)

	hexPattern := regexp.MustCompile(`^[0-9a-f]+$`)
	assert.True(t, hexPattern.MatchString(token), "token must be lowercase hex")

	session := store.Get(token)
	require.NotNil(t, session)

	assert.Equal(t, token, session.Token)
	assert.False(t, session.CreatedAt.IsZero(), "CreatedAt must be set")
	assert.False(t, session.ExpiresAt.IsZero(), "ExpiresAt must be set")
	assert.False(t, session.LastAccessed.IsZero(), "LastAccessed must be set")
	assert.True(t, session.ExpiresAt.After(session.CreatedAt), "ExpiresAt must be after CreatedAt")
}

func TestSessionStore_Get_NotFound(t *testing.T) {
	store := testSessionStore(t, time.Hour)

	session := store.Get("nonexistent-token")
	assert.Nil(t, session)
}

func TestSessionStore_Get_Expired(t *testing.T) {
	store := testSessionStore(t, 50*time.Millisecond)

	token, err := store.Create()
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	session := store.Get(token)
	assert.Nil(t, session, "expired session must return nil")
}

func TestSessionStore_Get_UpdatesLastAccessed(t *testing.T) {
	store := testSessionStore(t, time.Hour)

	token, err := store.Create()
	require.NoError(t, err)

	// Capture the initial LastAccessed via a direct Get right after creation.
	initial := store.Get(token)
	require.NotNil(t, initial)
	initialAccess := initial.LastAccessed

	time.Sleep(10 * time.Millisecond)

	updated := store.Get(token)
	require.NotNil(t, updated)

	assert.True(t, updated.LastAccessed.After(initialAccess),
		"LastAccessed must be updated on subsequent Get")
}

func TestSessionStore_Delete(t *testing.T) {
	store := testSessionStore(t, time.Hour)

	token, err := store.Create()
	require.NoError(t, err)

	store.Delete(token)

	session := store.Get(token)
	assert.Nil(t, session, "session must be nil after delete")
}

func TestSessionStore_Delete_Nonexistent(t *testing.T) {
	store := testSessionStore(t, time.Hour)

	// Must not panic.
	assert.NotPanics(t, func() {
		store.Delete("does-not-exist")
	})
}

func TestSessionStore_CleanupExpired(t *testing.T) {
	// Create 3 sessions with a very short TTL.
	shortStore := testSessionStore(t, 50*time.Millisecond)

	for range 3 {
		_, err := shortStore.Create()
		require.NoError(t, err)
	}

	// Create 2 sessions with a long TTL in the same short store won't work cleanly —
	// use two separate stores: migrate the long-TTL sessions into shortStore manually
	// by direct map injection so they share the same store for the cleanup assertion.
	shortStore.mu.Lock()
	for range 2 {
		token, err := generateToken()
		require.NoError(t, err)
		now := time.Now()
		shortStore.sessions[token] = &Session{
			Token:        token,
			CreatedAt:    now,
			ExpiresAt:    now.Add(10 * time.Second),
			LastAccessed: now,
		}
	}
	shortStore.mu.Unlock()

	// Wait for the short-TTL sessions to expire.
	time.Sleep(100 * time.Millisecond)

	removed := shortStore.CleanupExpired()
	assert.Equal(t, 3, removed, "exactly 3 short-TTL sessions must be cleaned up")

	// The 2 long-TTL sessions must still be accessible.
	shortStore.mu.Lock()
	remaining := len(shortStore.sessions)
	shortStore.mu.Unlock()
	assert.Equal(t, 2, remaining, "2 long-TTL sessions must remain")
}

func TestSessionStore_TokenUniqueness(t *testing.T) {
	store := testSessionStore(t, time.Hour)

	const count = 100
	tokens := make(map[string]struct{}, count)

	for range count {
		token, err := store.Create()
		require.NoError(t, err)
		tokens[token] = struct{}{}
	}

	assert.Len(t, tokens, count, "all %d tokens must be unique", count)
}

func TestSessionStore_Concurrent(t *testing.T) {
	store := testSessionStore(t, time.Hour)

	const workers = 50
	var wg sync.WaitGroup
	wg.Add(workers)

	for range workers {
		go func() {
			defer wg.Done()

			token, err := store.Create()
			if err != nil {
				return
			}

			session := store.Get(token)
			if session == nil {
				return
			}

			store.Delete(token)

			_ = store.Get(token)
		}()
	}

	wg.Wait()
}

func TestSessionStore_Stop_Idempotent(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := NewSessionStore(time.Hour, logger)

	// Both calls must not panic.
	assert.NotPanics(t, func() {
		store.Stop()
		store.Stop()
	})
}

func TestGenerateToken(t *testing.T) {
	token1, err := generateToken()
	require.NoError(t, err)

	// Must be exactly SessionTokenBytes*2 hex characters.
	assert.Len(t, token1, SessionTokenBytes*2)

	hexPattern := regexp.MustCompile(`^[0-9a-f]+$`)
	assert.True(t, hexPattern.MatchString(token1), "token must only contain hex chars [0-9a-f]")

	token2, err := generateToken()
	require.NoError(t, err)

	assert.NotEqual(t, token1, token2, "two generated tokens must differ")
}

func TestSessionTokenPrefix(t *testing.T) {
	tests := []struct {
		name     string
		token    string
		expected string
	}{
		{"normal token", "abcdef1234567890abcdef", "abcdef12"},
		{"exactly prefix length", "abcdef12", "abcdef12"},
		{"shorter than prefix", "abc", "abc"},
		{"empty token", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, sessionTokenPrefix(tt.token))
		})
	}
}

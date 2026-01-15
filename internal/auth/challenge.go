package auth

import (
	"crypto/rand"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ErrChallengeNotFound = errors.New("challenge not found")
	ErrChallengeExpired  = errors.New("challenge expired")
)

type Challenge struct {
	ID        string
	DeviceID  string
	Nonce     []byte
	ExpiresAt time.Time
}

type ChallengeStore struct {
	mu         sync.RWMutex
	challenges map[string]*Challenge
	ttl        time.Duration
	stopCh     chan struct{}
}

func NewChallengeStore(ttl time.Duration) *ChallengeStore {
	cs := &ChallengeStore{
		challenges: make(map[string]*Challenge),
		ttl:        ttl,
		stopCh:     make(chan struct{}),
	}
	go cs.cleanupLoop()
	return cs
}

func (cs *ChallengeStore) Stop() {
	close(cs.stopCh)
}

func (cs *ChallengeStore) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			cs.cleanup()
		case <-cs.stopCh:
			return
		}
	}
}

func (cs *ChallengeStore) cleanup() {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	now := time.Now()
	for id, c := range cs.challenges {
		if now.After(c.ExpiresAt) {
			delete(cs.challenges, id)
		}
	}
}

func (cs *ChallengeStore) Create(deviceID string) (*Challenge, error) {
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}

	challenge := &Challenge{
		ID:        uuid.NewString(),
		DeviceID:  deviceID,
		Nonce:     nonce,
		ExpiresAt: time.Now().Add(cs.ttl),
	}

	cs.mu.Lock()
	cs.challenges[challenge.ID] = challenge
	cs.mu.Unlock()

	return challenge, nil
}

func (cs *ChallengeStore) Consume(id string) (*Challenge, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	challenge, ok := cs.challenges[id]
	if !ok {
		return nil, ErrChallengeNotFound
	}
	delete(cs.challenges, id)

	if time.Now().After(challenge.ExpiresAt) {
		return nil, ErrChallengeExpired
	}
	return challenge, nil
}

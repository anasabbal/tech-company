package store

import (
	"log"
	"sync"
	"sync/atomic"
	"test/model"
)

// MessageStore holds messages in memory.
// RWMutex: reads (GET /messages) are far more frequent than writes (POST /messages)
// so we allow many concurrent reads but serialise writes.
type MessageStore struct {
	mu       sync.RWMutex
	messages []model.Message
	nextID   int64 // atomic counter — no mutex needed for just incrementing
}

func NewMessageStore() *MessageStore {
	return &MessageStore{
		messages: make([]model.Message, 0),
	}
}

func (s *MessageStore) Add(msg model.Message) model.Message {
	msg.ID = int(atomic.AddInt64(&s.nextID, 1))

	s.mu.Lock()
	defer s.mu.Unlock()

	s.messages = append(s.messages, msg)
	log.Printf("[Store] WRITE lock: added message #%d from '%s'", msg.ID, msg.From)
	return msg
}

func (s *MessageStore) GetAll() []model.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]model.Message, len(s.messages))
	copy(result, s.messages)
	log.Printf("[Store] READ lock: returning %d messages", len(result))
	return result
}

func (s *MessageStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.messages)
}

type ResultStore struct {
	mu      sync.RWMutex
	results []model.JobResult
	total   int64
}

func NewResultStore() *ResultStore {
	return &ResultStore{results: make([]model.JobResult, 0)}
}

func (r *ResultStore) Add(res model.JobResult) {
	atomic.AddInt64(&r.total, 1) // lock-free increment

	r.mu.Lock()
	defer r.mu.Unlock()
	r.results = append(r.results, res)
}

func (r *ResultStore) GetAll() []model.JobResult {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]model.JobResult, len(r.results))
	copy(out, r.results)
	return out
}

func (r *ResultStore) Total() int64 {
	return atomic.LoadInt64(&r.total) // lock-free read
}

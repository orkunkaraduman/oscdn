package namedlock

import (
	"context"
	"sync"
	"time"
)

type NamedLock struct {
	mu      sync.Mutex
	holders map[string]*holder
}

func New() *NamedLock {
	return &NamedLock{
		holders: make(map[string]*holder),
	}
}

func (n *NamedLock) Locker(name string) *Locker {
	return &Locker{
		owner: n,
		name:  name,
	}
}

func (n *NamedLock) holder(name string) *holder {
	n.mu.Lock()
	h := n.holders[name]
	n.mu.Unlock()
	return h
}

func (n *NamedLock) increase(name string) *holder {
	n.mu.Lock()
	h := n.holders[name]
	if h == nil {
		h = &holder{
			owner: n,
			name:  name,
		}
		n.holders[name] = h
	}
	h.count++
	n.mu.Unlock()
	return h
}

type holder struct {
	owner *NamedLock
	name  string
	count int
	mutex sync.Mutex
}

func (h *holder) decrease() {
	h.owner.mu.Lock()
	h.count--
	if h.count <= 0 {
		delete(h.owner.holders, h.name)
	}
	h.owner.mu.Unlock()
}

type Locker struct {
	owner *NamedLock
	name  string
}

func (l *Locker) Lock() {
	// increase count, get holder
	h := l.owner.increase(l.name)
	// lock
	h.mutex.Lock()
}

func (l *Locker) Unlock() {
	// get holder
	h := l.owner.holder(l.name)
	if h == nil {
		panic("namedlock: unlock of unlocked mutex")
	}
	// unlock
	h.mutex.Unlock()
	// decrease count, delete holder
	h.decrease()
}

func (l *Locker) TryLock() bool {
	// increase count, get holder
	h := l.owner.increase(l.name)
	// try  lock
	ok := h.mutex.TryLock()
	if !ok {
		// decrease count, delete holder if needed
		h.decrease()
	}
	return ok
}

func (l *Locker) AcquireLock(ctx context.Context) (err error) {
	for ctx.Err() == nil {
		if l.TryLock() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
	return ctx.Err()
}

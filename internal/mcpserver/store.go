package mcpserver

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/Sebas1705/WinForge/internal/mcpkit"
)

// Plan store defaults.
const (
	DefaultPlanTTL = 10 * time.Minute
	MaxPlans       = 32
)

// PlanInfo is what winforge_list_plans shows about a pending plan.
type PlanInfo struct {
	ID      string    `json:"planId"`
	Kind    string    `json:"kind"`
	Summary any       `json:"summary"`
	Admin   bool      `json:"needsAdmin"`
	Expires time.Time `json:"expires"`
}

type planEntry struct {
	plan    mcpkit.Plan
	expires time.Time
}

// PlanStore keeps plans in memory between prepare and apply. Ids are 128 random bits, a plan can be taken
// once, it expires after the TTL, and when more than MaxPlans are waiting the oldest is dropped. It is safe
// for concurrent use and implements mcpkit.PlanStore.
type PlanStore struct {
	mu    sync.Mutex
	ttl   time.Duration
	now   func() time.Time
	byID  map[string]*planEntry
	order []string // oldest first
}

var _ mcpkit.PlanStore = (*PlanStore)(nil)

// NewPlanStore returns an empty store. A zero ttl means DefaultPlanTTL; a nil now means time.Now.
func NewPlanStore(ttl time.Duration, now func() time.Time) *PlanStore {
	if ttl <= 0 {
		ttl = DefaultPlanTTL
	}
	if now == nil {
		now = time.Now
	}
	return &PlanStore{ttl: ttl, now: now, byID: map[string]*planEntry{}}
}

func newPlanID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("mcpserver: no randomness for a plan id: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}

// Put stores a plan and returns its id and expiry.
func (s *PlanStore) Put(p mcpkit.Plan) (string, time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked()
	id := newPlanID()
	exp := s.now().Add(s.ttl)
	s.byID[id] = &planEntry{plan: p, expires: exp}
	s.order = append(s.order, id)
	for len(s.order) > MaxPlans {
		delete(s.byID, s.order[0])
		s.order = s.order[1:]
	}
	return id, exp
}

// Take returns the plan and forgets it: a second Take of the same id fails.
func (s *PlanStore) Take(id string) (mcpkit.Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.byID[id]
	if !ok {
		return mcpkit.Plan{}, mcpkit.ErrPlanUnknown
	}
	s.removeLocked(id)
	if !s.now().Before(e.expires) {
		return mcpkit.Plan{}, mcpkit.ErrPlanUnknown
	}
	return e.plan, nil
}

// Discard forgets a plan without running it and reports whether it was pending.
func (s *PlanStore) Discard(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked()
	if _, ok := s.byID[id]; !ok {
		return false
	}
	s.removeLocked(id)
	return true
}

// List returns the pending plans, oldest first.
func (s *PlanStore) List() []PlanInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked()
	out := make([]PlanInfo, 0, len(s.order))
	for _, id := range s.order {
		e := s.byID[id]
		out = append(out, PlanInfo{ID: id, Kind: e.plan.Kind, Summary: e.plan.Summary, Admin: e.plan.Admin, Expires: e.expires})
	}
	return out
}

func (s *PlanStore) removeLocked(id string) {
	delete(s.byID, id)
	for i, o := range s.order {
		if o == id {
			s.order = append(s.order[:i], s.order[i+1:]...)
			break
		}
	}
}

func (s *PlanStore) pruneLocked() {
	now := s.now()
	for _, id := range append([]string(nil), s.order...) {
		if !now.Before(s.byID[id].expires) {
			s.removeLocked(id)
		}
	}
}

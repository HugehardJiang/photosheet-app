package jobs

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

type Phase string

const (
	PhaseScanning   Phase = "Scanning"
	PhaseHashing    Phase = "Hashing"
	PhasePlanning   Phase = "Planning"
	PhaseRendering  Phase = "Rendering"
	PhaseExtracting Phase = "Extracting"
)

type Status string

const (
	StatusRunning   Status = "running"
	StatusCancelled Status = "cancelled"
	StatusFailed    Status = "failed"
	StatusCompleted Status = "completed"
)

type Snapshot struct {
	ID        string    `json:"id"`
	Phase     Phase     `json:"phase"`
	Status    Status    `json:"status"`
	Processed int       `json:"processed"`
	Total     int       `json:"total"`
	Current   string    `json:"current"`
	Error     string    `json:"error,omitempty"`
	StartedAt time.Time `json:"startedAt"`
	EndedAt   time.Time `json:"endedAt,omitempty"`
}

type Manager struct {
	mu   sync.Mutex
	next uint64
	jobs map[string]*Handle
}

func NewManager() *Manager { return &Manager{jobs: make(map[string]*Handle)} }

type Handle struct {
	manager *Manager
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	snap    Snapshot
}

func (m *Manager) Start(parent context.Context, phase Phase, total int) (*Handle, context.Context, error) {
	if parent == nil {
		parent = context.Background()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, job := range m.jobs {
		if job.Snapshot().Status == StatusRunning {
			return nil, nil, errors.New("已有任务正在运行")
		}
	}
	m.next++
	id := fmt.Sprintf("job-%06d", m.next)
	ctx, cancel := context.WithCancel(parent)
	handle := &Handle{manager: m, ctx: ctx, cancel: cancel, snap: Snapshot{ID: id, Phase: phase, Status: StatusRunning, Total: total, StartedAt: time.Now().UTC()}}
	m.jobs[id] = handle
	return handle, ctx, nil
}

func (h *Handle) Context() context.Context { return h.ctx }

func (h *Handle) Update(processed int, current string) {
	h.mu.Lock()
	h.snap.Processed = processed
	h.snap.Current = current
	h.mu.Unlock()
}

func (h *Handle) Complete() {
	h.mu.Lock()
	if h.snap.Status == StatusRunning {
		h.snap.Status = StatusCompleted
		h.snap.EndedAt = time.Now().UTC()
	}
	h.mu.Unlock()
	h.cancel()
}

func (h *Handle) Fail(err error) {
	h.mu.Lock()
	if h.snap.Status == StatusRunning {
		h.snap.Status = StatusFailed
		h.snap.EndedAt = time.Now().UTC()
		if err != nil {
			h.snap.Error = err.Error()
		}
	}
	h.mu.Unlock()
	h.cancel()
}

func (h *Handle) Cancel() {
	h.mu.Lock()
	if h.snap.Status == StatusRunning {
		h.snap.Status = StatusCancelled
		h.snap.EndedAt = time.Now().UTC()
	}
	h.mu.Unlock()
	h.cancel()
}

func (h *Handle) Snapshot() Snapshot {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.snap
}

func (m *Manager) Snapshot(id string) (Snapshot, bool) {
	m.mu.Lock()
	job, ok := m.jobs[id]
	m.mu.Unlock()
	if !ok {
		return Snapshot{}, false
	}
	return job.Snapshot(), true
}

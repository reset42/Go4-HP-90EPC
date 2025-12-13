package reader

import (
	"context"
	"errors"
	"sync"
	"time"

	"hp90epc/logging"
	"hp90epc/model"
)

type Status struct {
	Port        string    `json:"port"`
	Baud        int       `json:"baud"`
	Connected   bool      `json:"connected"`
	LastFrameAt time.Time `json:"last_frame_at"`
	LastError   string    `json:"last_error"`
}

type Manager struct {
	mu sync.RWMutex

	latest *model.LatestBuffer
	logger *logging.Logger

	cancel  context.CancelFunc
	running bool

	staleAfter time.Duration
	status     Status
}

func NewManager(latest *model.LatestBuffer, logger *logging.Logger, stale time.Duration) *Manager {
	if stale <= 0 {
		stale = 3 * time.Second
	}
	return &Manager{
		latest:     latest,
		logger:     logger,
		staleAfter: stale,
		status:     Status{},
	}
}

func (m *Manager) GetStatus() Status {
	m.mu.RLock()
	st := m.status
	stale := m.staleAfter
	m.mu.RUnlock()

	// Connected NICHT "sticky" machen, sondern aus LastFrameAt ableiten
	if !st.LastFrameAt.IsZero() && time.Since(st.LastFrameAt) <= stale && st.LastError == "" {
		st.Connected = true
	} else {
		st.Connected = false
	}
	return st
}

func (m *Manager) setStatus(fn func(*Status)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	fn(&m.status)
}

func (m *Manager) Start(port string, baud int) error {
	m.mu.Lock()

	if m.running && m.cancel != nil {
		m.cancel()
		m.running = false
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.running = true
	m.status.Port = port
	m.status.Baud = baud
	m.status.Connected = false
	m.status.LastError = ""

	m.mu.Unlock()

	go func() {
		err := RunLoop(ctx, port, baud, m.latest, m.logger, func() {
			m.setStatus(func(s *Status) {
				s.LastFrameAt = time.Now()
				s.LastError = ""
			})
		})

		if err != nil && !errors.Is(err, context.Canceled) {
			m.setStatus(func(s *Status) {
				s.LastError = err.Error()
			})
		}
	}()

	return nil
}

func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		m.cancel()
	}
	m.running = false
	m.status.Connected = false
}

func (m *Manager) SetPort(port string, baud int) error {
	return m.Start(port, baud)
}


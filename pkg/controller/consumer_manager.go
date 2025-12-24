package controller

import (
	"context"
	"sync"
	"time"

	v1 "github.com/flanksource/batch-runner/pkg/apis/batch/v1"
	"github.com/flanksource/batch-runner/pkg"
	dutyctx "github.com/flanksource/duty/context"
	"k8s.io/apimachinery/pkg/types"
)

const (
	ConnectionStateConnected    = "Connected"
	ConnectionStateDisconnected = "Disconnected"
	ConnectionStateError        = "Error"
	ConnectionStateStarting     = "Starting"
)

type ConsumerStats struct {
	mu                sync.Mutex
	MessagesProcessed int64
	MessagesFailed    int64
	MessagesRetried   int64
	LastError         string
	LastErrorTime     time.Time
	ConnectionState   string
}

func (s *ConsumerStats) RecordProcessed() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.MessagesProcessed++
}

func (s *ConsumerStats) RecordFailed(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.MessagesFailed++
	s.LastError = err.Error()
	s.LastErrorTime = time.Now()
}

func (s *ConsumerStats) RecordRetried() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.MessagesRetried++
}

func (s *ConsumerStats) SetConnectionState(state string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ConnectionState = state
}

func (s *ConsumerStats) Snapshot() ConsumerStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return ConsumerStats{
		MessagesProcessed: s.MessagesProcessed,
		MessagesFailed:    s.MessagesFailed,
		MessagesRetried:   s.MessagesRetried,
		LastError:         s.LastError,
		LastErrorTime:     s.LastErrorTime,
		ConnectionState:   s.ConnectionState,
	}
}

type ManagedConsumer struct {
	cancel    context.CancelFunc
	config    *v1.Config
	stats     *ConsumerStats
	startedAt time.Time
}

type ConsumerManager struct {
	mu        sync.RWMutex
	consumers map[types.NamespacedName]*ManagedConsumer
	rootCtx   dutyctx.Context
}

func NewConsumerManager(rootCtx dutyctx.Context) *ConsumerManager {
	return &ConsumerManager{
		consumers: make(map[types.NamespacedName]*ManagedConsumer),
		rootCtx:   rootCtx,
	}
}

func (m *ConsumerManager) Start(key types.NamespacedName, config *v1.Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.consumers[key]; exists {
		return nil
	}

	ctx, cancel := context.WithCancel(m.rootCtx)
	stats := &ConsumerStats{
		ConnectionState: ConnectionStateStarting,
	}

	managed := &ManagedConsumer{
		cancel:    cancel,
		config:    config,
		stats:     stats,
		startedAt: time.Now(),
	}
	m.consumers[key] = managed

	callbacks := &pkg.ConsumerCallbacks{
		OnMessageProcessed: stats.RecordProcessed,
		OnMessageFailed:    stats.RecordFailed,
		OnMessageRetried:   stats.RecordRetried,
		OnConnectionChange: stats.SetConnectionState,
	}

	go func() {
		stats.SetConnectionState(ConnectionStateConnected)
		err := pkg.RunConsumerWithCallbacks(m.rootCtx.Wrap(ctx), config, callbacks)
		if err != nil && ctx.Err() == nil {
			stats.SetConnectionState(ConnectionStateError)
			stats.RecordFailed(err)
		} else {
			stats.SetConnectionState(ConnectionStateDisconnected)
		}
	}()

	return nil
}

func (m *ConsumerManager) Stop(key types.NamespacedName) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if managed, exists := m.consumers[key]; exists {
		managed.cancel()
		delete(m.consumers, key)
	}
}

func (m *ConsumerManager) IsRunning(key types.NamespacedName) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, exists := m.consumers[key]
	return exists
}

func (m *ConsumerManager) GetStats(key types.NamespacedName) *ConsumerStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if managed, exists := m.consumers[key]; exists {
		snapshot := managed.stats.Snapshot()
		return &snapshot
	}
	return &ConsumerStats{ConnectionState: ConnectionStateDisconnected}
}

func (m *ConsumerManager) UpdateConfig(key types.NamespacedName, newConfig *v1.Config) error {
	m.mu.Lock()
	managed, exists := m.consumers[key]
	m.mu.Unlock()

	if !exists {
		return m.Start(key, newConfig)
	}

	if configChanged(managed.config, newConfig) {
		m.Stop(key)
		return m.Start(key, newConfig)
	}

	return nil
}

func configChanged(old, new *v1.Config) bool {
	return old.String() != new.String()
}

func (m *ConsumerManager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for key, managed := range m.consumers {
		managed.cancel()
		delete(m.consumers, key)
	}
}

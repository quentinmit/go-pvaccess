package monitor

import (
	"context"
	"sync"

	"github.com/quentinmit/go-pvaccess/pvdata"
	"github.com/quentinmit/go-pvaccess/types"
)

type Monitor struct {
	sendValue  func(interface{})
	mu         sync.Mutex
	cancel     func()
	pipeline   bool
	running    bool
	windowOpen int
	toSend     interface{}
}

func New(ctx context.Context, request pvdata.PVStructure, nexter types.Nexter, sendValue func(interface{})) *Monitor {
	var pipeline bool
	field := request.SubField("record", "_options", "pipeline")
	if field, ok := pvdata.BoolValue(field); ok {
		pipeline = field
	}
	ctx, cancel := context.WithCancel(ctx)
	m := &Monitor{
		pipeline:  pipeline,
		sendValue: sendValue,
		cancel:    cancel,
	}
	go m.Watch(ctx, nexter)
	return m
}

func (m *Monitor) Watch(ctx context.Context, nexter types.Nexter) error {
	for {
		// Check if context is canceled in case Next doesn't use context.
		if err := ctx.Err(); err != nil {
			return err
		}
		value, err := nexter.Next(ctx)
		if err != nil {
			return err
		}
		m.Send(ctx, value)
	}
}

func (m *Monitor) Start(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.running = true
	m.drain()
}

func (m *Monitor) Stop(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.running = false
	m.drain()
}

func (m *Monitor) Ack(ctx context.Context, nfree int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.windowOpen += nfree
	m.drain()
}

func (m *Monitor) drain() {
	if m.running && (!m.pipeline || m.windowOpen > 0) && m.toSend != nil {
		if m.windowOpen > 0 {
			m.windowOpen--
		}
		m.sendValue(m.toSend)
		m.toSend = nil
	}
}

func (m *Monitor) Send(ctx context.Context, value interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toSend = value
	m.drain()
}

func (m *Monitor) Terminate(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cancel()
	// TODO: Send final update to client
	return nil
}

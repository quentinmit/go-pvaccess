package pvaccess

import (
	"context"
	"fmt"
	"sync"

	"github.com/quentinmit/go-pvaccess/internal/ctxlog"
	"github.com/quentinmit/go-pvaccess/internal/server/types"
	"github.com/quentinmit/go-pvaccess/pvdata"
	"golang.org/x/sync/errgroup"
)

type ChannelProvider = types.ChannelProvider
type ChannelLister = types.ChannelLister
type ChannelFinder = types.ChannelFinder
type Channel = types.Channel
type ChannelGetCreator = types.ChannelGetCreator
type ChannelGeter = types.ChannelGeter
type ChannelRPCCreator = types.ChannelRPCCreator
type ChannelRPCer = types.ChannelRPCer
type ChannelMonitorCreator = types.ChannelMonitorCreator
type Nexter = types.Nexter

func (conn *serverConn) createChannel(ctx context.Context, channelID pvdata.PVInt, name string) (Channel, error) {
	conn.mu.Lock()
	if _, ok := conn.channels[channelID]; ok {
		conn.mu.Unlock()
		return nil, fmt.Errorf("channel %d already created", channelID)
	}
	conn.mu.Unlock()
	g, ctx := errgroup.WithContext(ctx)
	var channel Channel
	conn.srv.mu.RLock()
	for _, provider := range conn.srv.channelProviders {
		provider := provider
		g.Go(func() error {
			c, err := provider.CreateChannel(ctx, name)
			if err != nil {
				ctxlog.L(ctx).Warnf("ChannelProvider %v failed to create channel %q: %v", provider, name, err)
				return nil
			}
			if c != nil {
				channel = c
				return context.Canceled
			}
			return nil
		})
	}
	conn.srv.mu.RUnlock()
	if err := g.Wait(); err != nil && err != context.Canceled {
		return nil, err
	}
	conn.mu.Lock()
	conn.channels[channelID] = channel
	conn.mu.Unlock()
	return channel, nil
}

func (c *serverConn) destroyChannel(id pvdata.PVInt) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	// TODO: Cancel outstanding requests?
	// TODO: Wait for outstanding requests to finish?
	if _, ok := c.channels[id]; ok {
		delete(c.channels, id)
		return nil
	}
	return fmt.Errorf("unknown channel %d", id)
}

type SimpleChannel struct {
	name string

	mu    sync.Mutex
	value interface{}
	seq   int
	cond  *sync.Cond
}

func NewSimpleChannel(name string) *SimpleChannel {
	c := &SimpleChannel{
		name: name,
	}
	c.cond = sync.NewCond(&c.mu)
	return c
}

func (c *SimpleChannel) Name() string {
	return c.name
}

// Get returns the current value in c.
func (c *SimpleChannel) Get() interface{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.value
}

// Set changes the value in c and notifies any clients that are monitoring the channel.
// It is not recommended to change the type of the value between calls to Set.
func (c *SimpleChannel) Set(value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.value = value
	c.seq++
	c.cond.Broadcast()
}

func (c *SimpleChannel) CreateChannel(ctx context.Context, name string) (Channel, error) {
	if c.Name() == name {
		return c, nil
	}
	return nil, nil
}
func (c *SimpleChannel) ChannelList(ctx context.Context) ([]string, error) {
	return []string{c.Name()}, nil
}

type watch struct {
	c   *SimpleChannel
	seq int
}

func (w *watch) Next(ctx context.Context) (interface{}, error) {
	w.c.mu.Lock()
	defer w.c.mu.Unlock()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		<-ctx.Done()
		w.c.cond.Broadcast()
	}()
	for w.seq >= w.c.seq {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		w.c.cond.Wait()
	}
	w.seq = w.c.seq
	return &bareScalar{
		Value: w.c.value,
	}, nil
}

func (c *SimpleChannel) CreateChannelMonitor(ctx context.Context, req pvdata.PVStructure) (types.Nexter, error) {
	return &watch{c, -1}, nil
}

type bareScalar struct {
	Value interface{} `pvaccess:"value"`
}

func (bareScalar) TypeID() string {
	return "epics:nt/NTScalar:1.0"
}

func (c *SimpleChannel) ChannelGet(ctx context.Context) (interface{}, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return &bareScalar{
		Value: c.value,
	}, nil
}

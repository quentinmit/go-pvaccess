package pvaccess

import (
	"context"
	"errors"
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
type ChannelRPCCreator = types.ChannelRPCCreator
type ChannelRPCer = types.ChannelRPCer

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

type SimpleChannel struct {
	ChannelName string

	mu    sync.Mutex
	value interface{}
}

func (c *SimpleChannel) Name() string {
	return c.ChannelName
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
	// TODO: Notify watchers
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

func (c *SimpleChannel) ChannelGet(ctx context.Context) (interface{}, error) {
	// TODO: Implement.
	return nil, errors.New("not implemented")
}

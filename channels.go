package pvaccess

import (
	"context"
	"fmt"

	"github.com/quentinmit/go-pvaccess/internal/ctxlog"
	"github.com/quentinmit/go-pvaccess/pvdata"
	"golang.org/x/sync/errgroup"
)

// ChannelProvider represents the minimal channel provider.
// Optionally, a channel provider may implement ChannelLister or ChannelFinder.
type ChannelProvider interface {
	CreateChannel(ctx context.Context, name string) (Channel, error)
}
type ChannelLister interface {
	ChannelList(ctx context.Context) ([]string, error)
}
type ChannelFinder interface {
	ChannelFind(ctx context.Context, name string) (bool, error)
}

// Channel represents the minimal channel.
// For a channel to be useful, it must implement one of the following additional interfaces:
// CreateChannelProcess
// CreateChannelGet
// CreateChannelPut
// CreateChannelPutGet
// CreateChannelRPC
// CreateMonitor
// CreateChannelArray
type Channel interface {
	Name() string
}

type ChannelRPCCreator interface {
	CreateChannelRPC(ctx context.Context, req pvdata.PVStructure) (ChannelRPCer, error)
}

type ChannelRPCer interface {
	ChannelRPC(ctx context.Context, req pvdata.PVStructure) (response interface{}, err error)
}

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

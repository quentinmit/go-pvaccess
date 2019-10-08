package pvaccess

import (
	"context"
	"fmt"

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

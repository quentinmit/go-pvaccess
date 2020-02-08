package types

import (
	"context"

	"go-pvaccess/pvdata"
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
//
// For a channel to be useful, it must implement one of the following additional interfaces:
//
// - CreateChannelProcess
// - CreateChannelGet
// - CreateChannelPut
// - CreateChannelPutGet
// - CreateChannelRPC
// - CreateMonitor
// - CreateChannelArray
type Channel interface {
	Name() string
}

type ChannelGetCreator interface {
	CreateChannelGet(ctx context.Context, req pvdata.PVStructure) (ChannelGeter, error)
}

type ChannelGeter interface {
	ChannelGet(ctx context.Context) (response interface{}, err error)
}

type ChannelRPCCreator interface {
	CreateChannelRPC(ctx context.Context, req pvdata.PVStructure) (ChannelRPCer, error)
}

type ChannelRPCer interface {
	ChannelRPC(ctx context.Context, req pvdata.PVStructure) (response interface{}, err error)
}

type ChannelMonitorCreator interface {
	CreateChannelMonitor(ctx context.Context, req pvdata.PVStructure) (Nexter, error)
}

type Nexter interface {
	Next(ctx context.Context) (interface{}, error)
}

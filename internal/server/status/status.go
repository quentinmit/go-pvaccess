package status

import (
	"context"
	"os"
	"runtime"
	"strings"

	"github.com/quentinmit/go-pvaccess/internal/ctxlog"
	"github.com/quentinmit/go-pvaccess/internal/server/types"
	"github.com/quentinmit/go-pvaccess/pvdata"
)

type ChannelProviderser interface {
	ChannelProviders() []types.ChannelProvider
}

type Channel struct {
	Server ChannelProviderser
}

func (Channel) Name() string {
	return "server"
}

func (c *Channel) CreateChannel(ctx context.Context, name string) (types.Channel, error) {
	if name == c.Name() {
		return c, nil
	}
	return nil, nil
}

type NTScalarArray struct {
	Value []string `pvaccess:"value"`
}

func (NTScalarArray) TypeID() string {
	return "epics:nt/NTScalarArray:1.0"
}

func (c *Channel) ChannelRPC(ctx context.Context, args pvdata.PVStructure) (interface{}, error) {
	if strings.HasPrefix(args.ID, "epics:nt/NTURI:1.") {
		if q, ok := args.Field("query").(*pvdata.PVStructure); ok {
			args = *q
		} else {
			return struct{}{}, pvdata.PVStatus{
				Type:    pvdata.PVStatus_ERROR,
				Message: pvdata.PVString("invalid argument"),
			}
		}
	}

	if args.Field("help") != nil {
		// TODO
	}

	var op pvdata.PVString
	if v, ok := args.Field("op").(*pvdata.PVString); ok {
		op = *v
	}

	ctxlog.L(ctx).Debugf("op = %s", op)

	switch op {
	case "channels":
		resp := &NTScalarArray{}
		// TODO: List channels in parallel
		for _, p := range c.Server.ChannelProviders() {
			if p, ok := p.(types.ChannelLister); ok {
				channels, err := p.ChannelList(ctx)
				if err != nil {
					ctxlog.L(ctx).Errorf("failed to list channels on %v", p)
					continue
				}
				resp.Value = append(resp.Value, channels...)
			}
		}
		return resp, nil
	case "info":
		hostname, _ := os.Hostname()
		info := &struct {
			Process   string `pvaccess:"process"`
			StartTime string `pvaccess:"startTime"`
			Version   string `pvaccess:"version"`
			ImplLang  string `pvaccess:"implLang"`
			Host      string `pvaccess:"host"`
			OS        string `pvaccess:"os"`
			Arch      string `pvaccess:"arch"`
		}{
			os.Args[0],
			"sometime",
			"1.0",
			"Go",
			hostname,
			runtime.GOOS,
			runtime.GOARCH,
		}
		ctxlog.L(ctx).Debugf("returning info %+v", info)
		return info, nil
	}

	return &struct{}{}, pvdata.PVStatus{
		Type:    pvdata.PVStatus_ERROR,
		Message: pvdata.PVString("invalid argument"),
	}
}

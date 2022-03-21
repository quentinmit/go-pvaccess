package search

import (
	"context"

	"github.com/quentinmit/go-pvaccess/internal/connection"
	"github.com/quentinmit/go-pvaccess/internal/ctxlog"
	"github.com/quentinmit/go-pvaccess/internal/proto"
	"github.com/quentinmit/go-pvaccess/pvdata"
	"github.com/quentinmit/go-pvaccess/types"
)

func (s *Server) Search(ctx context.Context, c *connection.Connection, req proto.SearchRequest) error {
	// TODO: When search is received over TCP, do we respond over TCP or do we respond over UDP?
	resp := &proto.SearchResponse{
		GUID:             s.GUID,
		SearchSequenceID: req.SearchSequenceID,
		ServerPort:       pvdata.PVUShort(s.ServerAddr.Port),
		Protocol:         "tcp",
		Found:            true,
	}
	copy(resp.ServerAddress[:], []byte(s.ServerAddr.IP.To16()))
	for _, p := range s.Server.ChannelProviders() {
		for _, channel := range req.Channels {
			if p, ok := p.(types.ChannelFinder); ok {
				present, err := p.ChannelFind(ctx, channel.ChannelName)
				if err != nil {
					ctxlog.L(ctx).Errorf("while attempting to find channel %q: %v", channel.ChannelName, err)
					continue
				}
				if present {
					resp.SearchInstanceIDs = append(resp.SearchInstanceIDs, channel.SearchInstanceID)
				}
				continue
			}
			c, err := p.CreateChannel(ctx, channel.ChannelName)
			if err != nil {
				ctxlog.L(ctx).Errorf("while attempting to create channel %q: %v", channel.ChannelName, err)
				continue
			}
			if c != nil {
				resp.SearchInstanceIDs = append(resp.SearchInstanceIDs, channel.SearchInstanceID)
			}
		}
	}
	if len(resp.SearchInstanceIDs) == 0 {
		resp.Found = false
		for _, channel := range req.Channels {
			resp.SearchInstanceIDs = append(resp.SearchInstanceIDs, channel.SearchInstanceID)
		}
	}
	if len(resp.SearchInstanceIDs) > 0 || req.Flags&proto.SEARCH_REPLY_REQUIRED == proto.SEARCH_REPLY_REQUIRED {
		if err := c.SendApp(ctx, proto.APP_SEARCH_RESPONSE, resp); err != nil {
			return err
		}
	}
	return nil
}

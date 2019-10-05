package search

import (
	"context"

	"github.com/quentinmit/go-pvaccess/internal/connection"
	"github.com/quentinmit/go-pvaccess/internal/proto"
	"github.com/quentinmit/go-pvaccess/pvdata"
)

func (s *Server) Search(ctx context.Context, c *connection.Connection, req proto.SearchRequest) error {
	// TODO: When search is received over TCP, do we respond over TCP or do we respond over UDP?
	resp := &proto.SearchResponse{
		GUID:             s.GUID,
		SearchSequenceID: req.SearchSequenceID,
		ServerPort:       pvdata.PVUShort(s.ServerAddr.Port),
		Protocol:         "tcp",
	}
	copy(resp.ServerAddress[:], []byte(s.ServerAddr.IP.To16()))
	var found []pvdata.PVUInt
	// TODO: Find channels
	if len(found) == 0 {
		resp.Found = false
		for _, channel := range req.Channels {
			resp.SearchInstanceIDs = append(resp.SearchInstanceIDs, channel.SearchInstanceID)
		}
	}
	if len(found) > 0 || req.Flags&proto.SEARCH_REPLY_REQUIRED == proto.SEARCH_REPLY_REQUIRED {
		if err := c.SendApp(ctx, proto.APP_SEARCH_RESPONSE, resp); err != nil {
			return err
		}
	}
	return nil
}

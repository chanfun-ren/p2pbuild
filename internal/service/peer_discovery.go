package service

import (
	"context"
	"github.com/chanfun-ren/executor/api"
	"github.com/chanfun-ren/executor/internal/network"
	"github.com/chanfun-ren/executor/pkg/logging"
)

type DiscoveryService struct {
	api.UnimplementedDiscoveryServer
	NetManager *network.NetManager
}

func NewDiscoveryService(netManager *network.NetManager) *DiscoveryService {
	return &DiscoveryService{
		NetManager: netManager,
	}
}

func (s *DiscoveryService) DiscoverPeers(ctx context.Context, req *api.DiscoverPeersRequest) (*api.DiscoverPeersResponse, error) {
	logging.DefaultLogger().Infof("Received DiscoverPeers request")

	peers := s.NetManager.PeerList()

	response := &api.DiscoverPeersResponse{
		Peers: make([]*api.Peer, len(peers)),
	}

	for i, p := range peers {
		response.Peers[i] = &api.Peer{
			Id:        p.ID,
			Addresses: p.Addresses,
		}
	}

	return response, nil
}

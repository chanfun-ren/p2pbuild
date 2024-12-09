package service

import (
	"context"

	"github.com/chanfun-ren/executor/api"
	"github.com/chanfun-ren/executor/internal/network"
	"github.com/chanfun-ren/executor/pkg/utils"
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
	peers := s.NetManager.PeerList()

	response := &api.DiscoverPeersResponse{
		Peers: make([]*api.Peer, len(peers)),
	}

	for i, p := range peers {
		ip, port, _ := utils.MaddrToHostPort(p.Addresses[0])
		response.Peers[i] = &api.Peer{
			Id:   p.ID,
			Ip:   ip,
			Port: port,
		}
	}

	return response, nil
}

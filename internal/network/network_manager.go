package network

import (
	"context"
	"sync"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	"github.com/multiformats/go-multiaddr"

	"github.com/chanfun-ren/executor/pkg/logging"
)

type Peer struct {
	ID        string
	Addresses []string
}

const (
	mdnsServiceTag = "example-mdns"
)

// PeerManager 抽象接口，用于管理活跃节点
type PeerManager interface {
	Add(peerID peer.ID, addresses []string)
	Remove(peerID peer.ID)
	List() []Peer
}

type ActivePeerStore struct {
	mu    sync.RWMutex
	peers map[peer.ID]Peer
}

func NewActivePeerStore() *ActivePeerStore {
	return &ActivePeerStore{
		peers: make(map[peer.ID]Peer),
	}
}

func (ap *ActivePeerStore) Add(peerID peer.ID, addresses []string) {
	ap.mu.Lock()
	defer ap.mu.Unlock()
	ap.peers[peerID] = Peer{
		ID:        peerID.String(),
		Addresses: addresses,
	}
}

func (ap *ActivePeerStore) Remove(peerID peer.ID) {
	ap.mu.Lock()
	defer ap.mu.Unlock()
	delete(ap.peers, peerID)
}

func (ap *ActivePeerStore) List() []Peer {
	ap.mu.RLock()
	defer ap.mu.RUnlock()
	result := make([]Peer, 0, len(ap.peers))
	for _, p := range ap.peers {
		result = append(result, p)
	}
	return result
}

type NetManager struct {
	mdns.Notifee
	network.Notifiee

	h           host.Host
	PeerManager PeerManager
}

var log = logging.DefaultLogger()

func (nm *NetManager) Connected(n network.Network, c network.Conn) {
	log.Infow("Peer connected", "peer", c.RemotePeer())
	nm.PeerManager.Add(c.RemotePeer(), []string{c.RemoteMultiaddr().String()})
}

func (nm *NetManager) Disconnected(n network.Network, c network.Conn) {
	log.Warnw("Peer disconnected", "peer", c.RemotePeer())
	nm.PeerManager.Remove(c.RemotePeer())
}

func (nm *NetManager) Listen(n network.Network, maddr multiaddr.Multiaddr) {
	log.Infow("Now listening on", "address", maddr)
}
func (nn *NetManager) ListenClose(n network.Network, maddr multiaddr.Multiaddr) {
	log.Infow("No longer listening on", "address", maddr)
}
func (nm *NetManager) HandlePeerFound(peer peer.AddrInfo) {
	log.Infow("Discovered peer", "peer", peer)
	if peer.ID > nm.h.ID() {
		// if other end peer id greater than us, don't connect to it, just wait for it to connect us
		// log.Infow("Found peer which id is greater than us, wait for it to connect to us", "peer", peer, "action", "wait")
		log.Debugw("Found greater peer", "peer", peer, "action", "wait")
		return
	}

	// log.Infow("Found peer which id is less than us, connect to it", "peer", peer, "action", "connect")
	log.Debugw("Found smaller peer", "peer", peer, "action", "connect")
	if err := nm.h.Connect(context.Background(), peer); err != nil {
		log.Warnw("Failed to connect to peer", "peer", peer, "error", err)
	}
}

func (nm *NetManager) PeerList() []Peer {
	return nm.PeerManager.List()
}

func NewNetManager(host host.Host) *NetManager {
	activePeerStore := NewActivePeerStore()

	// 自身上线, 将信息添加到 peerstore
	selfID := host.ID()
	selfAddrs := host.Addrs()
	addresses := make([]string, len(selfAddrs))
	for i, addr := range selfAddrs {
		addresses[i] = addr.String()
	}
	activePeerStore.Add(selfID, addresses)

	nm := &NetManager{
		h:           host,
		PeerManager: activePeerStore,
	}

	host.Network().Notify(nm)

	// setup mDNS discovery to find local peers
	s := mdns.NewMdnsService(host, mdnsServiceTag, nm)
	err := s.Start()
	if err != nil {
		log.Fatalf("Failed to setup mDNS: %v", err)
	}

	return nm
}

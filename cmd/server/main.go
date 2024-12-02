package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/chanfun-ren/executor/api"
	"github.com/chanfun-ren/executor/internal/network"
	"github.com/chanfun-ren/executor/internal/service"
	"github.com/chanfun-ren/executor/pkg/interceptor"
	"github.com/chanfun-ren/executor/pkg/logging"
	"github.com/chanfun-ren/executor/pkg/utils"
	"github.com/libp2p/go-libp2p"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
)

func main() {
	log := logging.DefaultLogger()

	// metrics
	go func() {
		http.Handle("/debug/metrics/prometheus", promhttp.Handler())
		log.Infoln("Starting metrics server: http://localhost:5001/debug/metrics/prometheus")
		log.Fatal(http.ListenAndServe(":5001", nil))
	}()

	// create a new libp2p Host that listens on a random TCP port
	addr := fmt.Sprintf("/ip4/%s/tcp/0", utils.GetOutboundIP())
	h, err := libp2p.New(libp2p.ListenAddrStrings(addr))
	if err != nil {
		log.Fatalw("Failed to create p2p host", "addr", addr, "error", err)
	}
	log.Infow("Starting executor", "self_peer_id", h.ID(), "addrs", h.Addrs())

	// setup network manager, 维护网络中活跃的节点
	nm := network.NewNetManager(h)

	// 对外提供 DiscoveryService 接口
	discoveryService := service.NewDiscoveryService(nm)
	grpcServer := grpc.NewServer(grpc.ChainUnaryInterceptor(interceptor.LogInterceptor, interceptor.MetricsInterceptor))
	api.RegisterDiscoveryServer(grpcServer, discoveryService)

	address := ":50051"
	lis, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatalw("grpc server failed to bind addr", "address", address, "error", err)
	}

	go func() {
		log.Info("starting grpc health server", "address", address)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalw("failed to serve grpc service", "error", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	<-stop
	log.Infoln("Shutting down...")

	// close grpc server, close peer connections
	log.Infoln("Closing grpc server")
	grpcServer.GracefulStop()
	log.Infoln("Closing host")
	if err := h.Close(); err != nil {
		log.Warnw("Failed to close host", "error", err)
	} else {
		log.Info("Host closed")
	}
}

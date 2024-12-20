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
	"github.com/chanfun-ren/executor/pkg/config"

	"github.com/chanfun-ren/executor/internal/store"
	"github.com/chanfun-ren/executor/pkg/interceptor"
	"github.com/chanfun-ren/executor/pkg/logging"
	"github.com/chanfun-ren/executor/pkg/utils"
	"github.com/libp2p/go-libp2p"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
)

func initDefaultRedisCli() store.KVStoreClient {
	redisConfig := store.KVStoreConfig{
		Type: "redis",
		Host: "localhost",
		Port: config.STORE_PORT,
	}
	redisCli, err := store.GetKVStoreFactory().CreateKVStoreClient(redisConfig)
	if err != nil {
		panic(fmt.Sprintf("Failed to create redis client: %v, config: %v", err, redisConfig))

	}
	return redisCli
}

func main() {
	log := logging.NewComponentLogger("main_server")

	// metrics
	go func() {
		http.Handle("/debug/metrics/prometheus", promhttp.Handler())
		log.Info("Starting metrics server: http://localhost:5001/debug/metrics/prometheus")
		if err := http.ListenAndServe(":5001", nil); err != nil {
			log.Errorw("metrics server failed", "error", err)
		}
	}()

	// create a new libp2p Host that listens on a random TCP port
	ip := utils.GetOutboundIP().String()
	addr := fmt.Sprintf("/ip4/%s/tcp/0", ip)
	h, err := libp2p.New(libp2p.ListenAddrStrings(addr))
	if err != nil {
		log.Fatalw("Failed to create p2p host", "addr", addr, "error", err)
	}
	log.Infow("Starting executor", "self_peer_id", h.ID(), "addrs", h.Addrs())

	// setup network manager, 维护网络中活跃的节点
	nm := network.NewNetManager(h)

	// 对外提供 DiscoveryService, ProxyService, ExecutorService API
	redisCli := initDefaultRedisCli()
	discoveryService := service.NewDiscoveryService(nm)
	sharebuildService := service.NewSharebuildProxyService(nm, redisCli)
	executroService := service.NewShareBuildExecutorService(h)
	grpcServer := grpc.NewServer(grpc.ChainUnaryInterceptor(interceptor.LogInterceptor, interceptor.MetricsInterceptor))
	api.RegisterDiscoveryServer(grpcServer, discoveryService)
	api.RegisterShareBuildProxyServer(grpcServer, sharebuildService)
	api.RegisterShareBuildExecutorServer(grpcServer, executroService)

	address := fmt.Sprintf(":%d", config.GRPC_PORT)
	lis, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatalw("grpc server failed to bind addr", "address", address, "error", err)
	}

	go func() {
		log.Infow("starting grpc health server", "address", address)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalw("failed to serve grpc service", "error", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	done := make(chan bool, 1)
	go func() {
		<-stop
		log.Info("Shutting down...")

		log.Info("Closing grpc server")
		grpcServer.GracefulStop()

		log.Info("Closing host")
		if err := h.Close(); err != nil {
			log.Warnw("Failed to close host", "error", err)
		} else {
			log.Info("Host closed")
		}

		done <- true
	}()

	<-done
	log.Info("Server stopped")
}

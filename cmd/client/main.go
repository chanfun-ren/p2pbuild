package main

import (
	"context"
	"fmt"

	"github.com/chanfun-ren/executor/api"
	"github.com/chanfun-ren/executor/pkg/logging"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	var log = logging.DefaultLogger()
	ctx := context.Background()

	server_addr := "localhost:50051"
	client_conn, err := grpc.Dial(server_addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		log.Fatalw("Failed to connect", "server_addr", server_addr, "error", err)
	}
	defer client_conn.Close()

	client := api.NewDiscoveryClient(client_conn)
	resp, err := client.DiscoverPeers(ctx, &api.DiscoverPeersRequest{})
	if err != nil {
		log.Fatalw("Failed to call DiscoverPeers", "error", err)
	}
	fmt.Println(resp)
}

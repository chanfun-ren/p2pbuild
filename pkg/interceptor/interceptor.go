// pkg/interceptor/interceptors.go
package interceptor

import (
	"context"
	"runtime/debug"
	"time"

	"github.com/chanfun-ren/executor/pkg/logging"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	// 定义一个全局的 HistogramVec 指标
	requestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "grpc_request_duration_seconds",
			Help:    "Duration of gRPC requests",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method"},
	)
)

func init() {
	prometheus.MustRegister(requestDuration)
}

func LogInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	log := logging.FromContext(ctx)
	log.Infow("Received request", "method", info.FullMethod, "request", req)
	start := time.Now()
	resp, err := handler(ctx, req)
	if err != nil {
		log.Errorw("Failed to handle request", "method", info.FullMethod, "error", err)
	}
	log.Infow("Handled request", "method", info.FullMethod, "duration", time.Since(start))
	return resp, err
}

func MetricsInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	start := time.Now()
	resp, err := handler(ctx, req)
	duration := time.Since(start).Seconds()

	requestDuration.WithLabelValues(info.FullMethod).Observe(duration)

	return resp, err
}

func RecoveryInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
	defer func() {
		log := logging.FromContext(ctx)
		if r := recover(); r != nil {
			log.Errorf("panic in RPC %s: %v\n%s", info.FullMethod, r, debug.Stack())
			err = status.Errorf(codes.Internal, "internal panic")
		}
	}()
	return handler(ctx, req)
}

// 然后 grpc.NewServer(grpc.UnaryInterceptor(recoveryInterceptor))

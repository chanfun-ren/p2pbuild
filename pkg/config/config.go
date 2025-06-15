package config

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/chanfun-ren/executor/pkg/logging"
)

// Config contains all configuration options for the application
type Config struct {
	// Server settings
	GrpcPort int `json:"grpc_port" yaml:"grpc_port"`

	// Storage settings
	StoreHost string        `json:"store_host" yaml:"store_host"`
	StorePort int           `json:"store_port" yaml:"store_port"`
	CmdTTL    time.Duration `json:"cmd_ttl" yaml:"cmd_ttl"`
	TaskTTL   time.Duration `json:"task_ttl" yaml:"task_ttl"`

	// Execution settings
	ExecutorHome string `json:"executor_home" yaml:"executor_home"`
	PoolSize     int    `json:"pool_size" yaml:"pool_size"`
	QueueSize    int    `json:"queue_size" yaml:"queue_size"`
}

// Global exported configuration (capitalized for export)
var GlobalConfig Config
var log = logging.DefaultLogger()

// Initialize configuration when package is imported
func init() {
	// Set default values
	GlobalConfig = Config{
		GrpcPort:     50051,
		StoreHost:    "localhost",
		StorePort:    6379,
		CmdTTL:       20 * time.Minute,
		TaskTTL:      20 * time.Minute,
		ExecutorHome: determineDefaultExecutorHome(),
		PoolSize:     int(float64(runtime.NumCPU()) * 2), // 1.5x CPU cores
		QueueSize:    1024 * 512,
	}

	// Override with environment variables
	if port := getEnvInt("GRPC_PORT", GlobalConfig.GrpcPort); port != GlobalConfig.GrpcPort {
		GlobalConfig.GrpcPort = port
	}

	if host := os.Getenv("STORE_HOST"); host != "" {
		GlobalConfig.StoreHost = host
	}

	if port := getEnvInt("STORE_PORT", GlobalConfig.StorePort); port != GlobalConfig.StorePort {
		GlobalConfig.StorePort = port
	}

	if ttl := getEnvDuration("CMD_TTL", GlobalConfig.CmdTTL); ttl != GlobalConfig.CmdTTL {
		GlobalConfig.CmdTTL = ttl
	}

	if ttl := getEnvDuration("TASK_TTL", GlobalConfig.TaskTTL); ttl != GlobalConfig.TaskTTL {
		GlobalConfig.TaskTTL = ttl
	}

	if home := os.Getenv("EXECUTOR_HOME"); home != "" {
		GlobalConfig.ExecutorHome = home
	}

	if size := getEnvInt("POOL_SIZE", GlobalConfig.PoolSize); size != GlobalConfig.PoolSize {
		GlobalConfig.PoolSize = size
	}

	if size := getEnvInt("QUEUE_SIZE", GlobalConfig.QueueSize); size != GlobalConfig.QueueSize {
		GlobalConfig.QueueSize = size
	}

	log.Infow("Configuration loaded",
		"storeHost", GlobalConfig.StoreHost,
		"grpcPort", GlobalConfig.GrpcPort,
		"poolSize", GlobalConfig.PoolSize)
}

// Helper functions
func determineDefaultExecutorHome() string {
	username := os.Getenv("USER")
	if username == "" {
		return "/home/executor"
	}
	return fmt.Sprintf("/home/%s/", username)
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
		log.Warnw("Invalid integer value in environment variable", "key", key, "value", value)
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
		log.Warnw("Invalid duration value in environment variable", "key", key, "value", value)
	}
	return defaultValue
}

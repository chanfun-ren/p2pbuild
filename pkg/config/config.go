package config

import (
	"fmt"
	"os"

	"github.com/chanfun-ren/executor/pkg/logging"
)

func GetExecutorHome() string {
	username := os.Getenv("USER")
	if username == "" {
		logging.DefaultLogger().Infow("Failed to get USER environment variable", "USER", username)
		return "executor"
	}
	executorHome := fmt.Sprintf("/home/%s/", username)
	return executorHome
}

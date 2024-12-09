package utils

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/chanfun-ren/executor/pkg/logging"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
)

var log = logging.DefaultLogger()

// Get preferred outbound ip of this machine
func GetOutboundIP() net.IP {
	var log = logging.DefaultLogger()
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		log.Fatalw("Failed to get outbound IP", "error", err)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return localAddr.IP
}

// eg: maddr := "/ip4/198.18.0.1/tcp/43480"
func MaddrToHostPort(maddr string) (string, int32, error) {
	parts := strings.Split(maddr, "/")
	if len(parts) < 5 || parts[0] != "" || (parts[1] != "ip4" && parts[1] != "ip6") || (parts[3] != "tcp" && parts[1] != "udp") {
		return "", 0, errors.New("invalid address format")
	}

	ip := parts[2]
	portStr := parts[4]
	// 将字符串 Port 转换为 int32
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, fmt.Errorf("invalid port: %v", err)
	}

	if port < 0 || port > 65535 {
		return "", 0, errors.New("port number out of range")
	}

	return ip, int32(port), nil
}

func PullImageIfNecessary(ctx context.Context, imageAddr string) error {
	if imageAddr == "" {
		log.Infow("no needed image to pull")
		return nil
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("error creating Docker client: %w", err)
	}

	// 尝试拉取镜像
	reader, err := cli.ImagePull(ctx, imageAddr, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("error pulling image: %w", err)
	}
	defer reader.Close()

	// 处理拉取过程中的输出
	_, err = io.Copy(os.Stdout, reader)
	if err != nil {
		return fmt.Errorf("error reading image pull output: %w", err)
	}

	log := logging.FromContext(ctx)
	log.Infow("image pulled successfully", "image_addr", imageAddr)
	return nil
}

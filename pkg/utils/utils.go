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
	"github.com/distribution/reference"

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
	log.Debugw("Try to Pull image", "imageAddr", imageAddr)
	if imageAddr == "" {
		log.Infow("No image to pull")
		return nil
	}

	// 标准化镜像地址
	normalizedImage, err := normalizeImage(imageAddr)
	if err != nil {
		return fmt.Errorf("failed to normalize image address: %w", err)
	}
	log.Debugw("Normalized image address", "normalizedImage", normalizedImage.String())

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("error creating Docker client: %w", err)
	}

	// 检查镜像是否已经存在
	images, err := cli.ImageList(ctx, image.ListOptions{})
	if err != nil {
		return fmt.Errorf("error listing images: %w", err)
	}
	for _, img := range images {
		for _, tag := range img.RepoTags {
			// 使用 reference.ParseNormalizedNamed 解析本地镜像地址
			localRef, err := reference.ParseNormalizedNamed(tag)
			if err != nil {
				log.Warnw("Failed to parse local image tag", "tag", tag, "error", err)
				continue
			}
			// 对比标准化的 Named 类型
			if reference.FamiliarString(localRef) == reference.FamiliarString(normalizedImage) {
				log.Infow("Image already exists, skipping pull", "image", normalizedImage.String())
				return nil
			}
		}
	}

	// 开始拉取镜像
	reader, err := cli.ImagePull(ctx, normalizedImage.String(), image.PullOptions{})
	if err != nil {
		return fmt.Errorf("error pulling image: %w", err)
	}
	defer reader.Close()

	// 处理拉取输出
	_, err = io.Copy(os.Stdout, reader)
	if err != nil {
		return fmt.Errorf("error reading image pull output: %w", err)
	}

	log.Infow("Image pulled successfully", "image_addr", normalizedImage.String())
	return nil
}

func normalizeImage(imageAddr string) (reference.NamedTagged, error) {
	// 使用 reference.ParseNormalizedNamed 标准化并确保带有 tag
	parsedRef, err := reference.ParseNormalizedNamed(imageAddr)
	if err != nil {
		return nil, fmt.Errorf("invalid image address: %w", err)
	}
	// 检查是否已经是 NamedTagged 类型
	if namedTagged, ok := parsedRef.(reference.NamedTagged); ok {
		return namedTagged, nil
	}
	// 否则添加默认 tag :latest
	return reference.WithTag(parsedRef, "latest")
}

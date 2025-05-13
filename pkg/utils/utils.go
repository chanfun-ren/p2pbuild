package utils

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chanfun-ren/executor/pkg/config"
	"github.com/chanfun-ren/executor/pkg/logging"
	"github.com/distribution/reference"

	contype "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
)

var log = logging.DefaultLogger()

func IsLocalExecutor(exexcutorIP string) bool {
	return strings.HasPrefix(exexcutorIP, GetOutboundIP().String())
}

func CanExexcuteRemotely(cmd string) bool {
	supportedCommands := []string{"gcc ", "g++ ", "c++ ", "clang ", "clang++ ", "javac "}

	for _, compiler := range supportedCommands {
		if strings.Contains(strings.ToLower(cmd), strings.ToLower(compiler)) {
			return true
		}
	}
	return false
}

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

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("error creating Docker client: %w", err)
	}

	// 标准化镜像地址
	normalizedImage, err := normalizeImage(imageAddr)
	if err != nil {
		return fmt.Errorf("failed to normalize image address: %w", err)
	}
	log.Debugw("Normalized image address", "normalizedImage", normalizedImage.String())

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
				log.Infow("Image already exists, skipping pull", "normalizedImage.String()", normalizedImage.String())
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

	log.Infow("Image pulled successfully", "normalizedImage.String()", normalizedImage.String())
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

// 将 sync.Map 转换为调试友好的格式
func MapToString(m *sync.Map) string {
	var result []string
	m.Range(func(key, value any) bool {
		result = append(result, fmt.Sprintf("%v: %v", key, value))
		return true
	})
	return strings.Join(result, ", ")
}

// TODO: container 执行的方式，将 projRootDir 作为映射目录
func MapWorkspace(mountedDir, projRootDir string) error {
	// 本地生成一个和客户端项目主目录同名路径，将其映射到挂载路径
	// 兼容客户端待编译文件内容中包含的头文件为绝对路径的情况
	// sudo mkdir -p /home/lab0/src_code/qt-everywhere-src-6.8.0
	// sudo mount --bind /home/executor/.ninja-mounts/qt-everywhere-src-6.8.0 /home/lab0/src_code/qt-everywhere-src-6.8.0
	os.MkdirAll(projRootDir, os.ModePerm)
	cmd := &Command{
		Content: fmt.Sprintf("mount --bind %s %s", mountedDir, projRootDir),
		WorkDir: "",
		Env:     nil,
	}

	res := ExecCommand(context.Background(), cmd)
	if res.ExitCode != 0 {
		return fmt.Errorf("failed to map workspace: %s", res.Stderr)
	}
	return nil
}

func ClearWorkspace(projRootDir string) error {
	cmd := &Command{
		Content: fmt.Sprintf("umount %s", projRootDir),
		WorkDir: "",
		Env:     nil,
	}

	res := ExecCommand(context.Background(), cmd)
	if res.ExitCode != 0 {
		return fmt.Errorf("failed to clear workspace: %s", res.Stderr)
	}
	return nil
}

// TODO: NFS操作：目录被其他进程打开时，处理 `device is busy` 错误
// MountNFS 挂载 NFS 共享目录
func MountNFS(ctx context.Context, host, srcDir, dstDir string) error {
	// Check if running as root
	if os.Geteuid() != 0 {
		return fmt.Errorf("mount requires root privileges")
	}

	cmd := &Command{
		Content: fmt.Sprintf("mount -t nfs -o async %s:%s %s", host, srcDir, dstDir),
		// Content: fmt.Sprintf("mount -t nfs -o rw,async,ac,acregmin=1,acregmax=3,acdirmin=1,acdirmax=3 %s:%s %s", host, srcDir, dstDir),
		WorkDir: "",
		Env:     nil,
	}

	// Execute mount with 30s timeout
	mountCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	result := ExecCommand(mountCtx, cmd)
	if result.ExitCode != 0 {
		return fmt.Errorf("mount nfs failed: %s (stderr: %s)",
			result.Error, result.Stderr)
	}

	return nil
}

func GenMountedRootDir(ninjaHost string, projRootDir string) string {
	rootDir := strings.TrimSuffix(projRootDir, "/")
	return filepath.Join(config.ExecutorHome, ninjaHost, rootDir)
}

func GetMountedBuildDir(ninjaHost string, ninjaBuildDir string) string {
	return filepath.Join(config.ExecutorHome, ninjaHost, ninjaBuildDir)
}

// UnmountNFS 取消挂载 NFS 共享目录
func UnmountNFS(ctx context.Context, localMountPoint string) error {
	// 拼接 umount 命令
	umountCmd := &Command{
		Content: fmt.Sprintf("umount %s", localMountPoint),
	}

	// 执行取消挂载命令
	result := ExecCommand(ctx, umountCmd)
	if result.ExitCode != 0 {
		return fmt.Errorf("unmount nfs failed: %s", result.Stderr)
	}

	return nil
}

// RemoveImageAndContainers 移除指定镜像及其派生的所有容器
func RemoveImageAndContainers(ctx context.Context, imageName string) error {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("failed to create docker client: %w", err)
	}
	defer cli.Close()

	containers, err := cli.ContainerList(ctx, contype.ListOptions{All: true})
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	// 移除与指定镜像相关的所有容器
	for _, container := range containers {
		if container.Image == imageName {
			if err := cli.ContainerStop(ctx, container.ID, contype.StopOptions{}); err != nil {
				log.Warnf("Failed to stop container %s: %v", container.ID, err)
			}

			if err := cli.ContainerRemove(ctx, container.ID, contype.RemoveOptions{Force: true}); err != nil {
				log.Warnf("Failed to remove container %s: %v", container.ID, err)
			} else {
				log.Infof("Removed container %s", container.ID)
			}
		}
	}

	_, err = cli.ImageRemove(ctx, imageName, image.RemoveOptions{Force: true})
	if err != nil {
		return fmt.Errorf("failed to remove image %s: %w", imageName, err)
	}

	log.Infof("Removed image %s and its associated containers", imageName)
	return nil
}

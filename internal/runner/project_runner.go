package runner

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/chanfun-ren/executor/api"
	"github.com/chanfun-ren/executor/pkg/logging"
	"github.com/chanfun-ren/executor/pkg/utils"
)

type ProjectRunner interface {
	PrepareEnvironment(ctx context.Context, request *api.PrepareLocalEnvRequest) error
	RunTask(command string) (string, error) // 执行编译任务
	Cleanup() error                         // 清理资源
}

var log = logging.DefaultLogger()

type LocalProjectRunner struct {
}

func NewLocalProjectRunner() *LocalProjectRunner {
	return &LocalProjectRunner{}
}

func (l *LocalProjectRunner) PrepareEnvironment(ctx context.Context, req *api.PrepareLocalEnvRequest) error {
	log.Infow("Preparing local environment for project", "project", req.String())
	return mountNinjaProject(ctx, req)
}

func mountNinjaProject(ctx context.Context, req *api.PrepareLocalEnvRequest) error {
	// 设置本地编译环境，例如 NFS 挂载
	project := req.GetProject()
	ninjaHost := project.GetNinjaHost()
	rootDir := strings.TrimSuffix(project.GetRootDir(), "/") //把结尾的斜杠去掉，方便后面字符串替换

	// 生成挂载点目录, 若目录不存在则递归创建
	mountedRootDir := utils.GenMountedRootDir(ninjaHost, rootDir)
	if err := os.MkdirAll(mountedRootDir, os.ModePerm); err != nil {
		log.Fatalw("fail to create dir", "mountedRootDir", mountedRootDir, "err", err)
	}
	log.Debugw("directory created successfully", "mountedRootDir", mountedRootDir)

	// 执行挂载 NFS 操作
	return utils.MountNFS(ctx, ninjaHost, rootDir, mountedRootDir)
}

func (l *LocalProjectRunner) RunTask(command string) (string, error) {
	fmt.Printf("Executing local task: %s\n", command)
	// 实际执行任务逻辑
	return "Build successful", nil
}

func (l *LocalProjectRunner) Cleanup() error {
	fmt.Println("Cleaning up local resources")
	return nil
}

type ContainerProjectRunner struct {
	containerImage string
	containerID    string
}

func NewContainerProjectRunner(image string) *ContainerProjectRunner {
	return &ContainerProjectRunner{
		containerImage: image,
	}
}

func (c *ContainerProjectRunner) PrepareEnvironment(ctx context.Context, req *api.PrepareLocalEnvRequest) error {
	log.Infow("Preparing container environment for project", "project", req.String())

	// 1. 挂载项目
	err := mountNinjaProject(ctx, req)
	if err != nil {
		log.Errorw("Failed to mount project", "req", req, "error", err)
		return err
	}

	// 2. 后台拉取镜像
	go func(image string) {
		bgCtx := context.Background() // 独立上下文，不会受请求的 ctx 限制
		if err := utils.PullImageIfNecessary(bgCtx, image); err != nil {
			log.Errorw("Failed to pull image in background", "image", image, "error", err)
		} else {
			log.Infow("Image pulled successfully in background", "image", image)
		}
	}(c.containerImage)

	// 3. 提前返回，拉取任务在后台进行
	return nil

}

func (c *ContainerProjectRunner) RunTask(command string) (string, error) {
	fmt.Printf("Executing task in container %s: %s\n", c.containerID, command)
	// 容器内执行任务
	return "Build successful (container)", nil
}

func (c *ContainerProjectRunner) Cleanup() error {
	fmt.Printf("Stopping and cleaning up container: %s\n", c.containerID)
	return nil
}

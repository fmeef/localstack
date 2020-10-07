package stack

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strconv"
	"syscall"
	"time"

	"github.com/containers/buildah"
	"github.com/containers/buildah/imagebuildah"
	"github.com/containers/podman/v2/libpod/define"
	"github.com/containers/podman/v2/pkg/bindings"
	"github.com/containers/podman/v2/pkg/bindings/containers"
	"github.com/containers/podman/v2/pkg/bindings/images"
	"github.com/containers/podman/v2/pkg/domain/entities"
	"github.com/containers/podman/v2/pkg/specgen"
	"github.com/containers/storage/pkg/archive"
	"github.com/jhoonb/archivex"
	log "github.com/sirupsen/logrus"
	"github.io/gnu3ra/localstack/buildtemplates"
	"github.io/gnu3ra/localstack/utils"
)

const (
	imageTag = "localstack-build-image"
	sockPath = "/tmp/localstack.sock"
	containerName = "localstack-build"
)

type DockerStackConfig struct {
	Name                   string
	Device                 string
	Email                  string
	SSHKey                 string
	Version                string
	Schedule               string
	ChromiumVersion        string
	CustomPatches          *utils.CustomPatches
	CustomScripts          *utils.CustomScripts
	CustomPrebuilts        *utils.CustomPrebuilts
	CustomManifestRemotes  *utils.CustomManifestRemotes
	CustomManifestProjects *utils.CustomManifestProjects
	HostsFile              string
	EnableAttestation      bool
	StatePath              string
	NumProc                int
}


type DockerStack struct {
	config *DockerStackConfig
	renderedBuildScript []byte
	buildScriptFileLocation string
	ctx context.Context
	statePath string
	podmanProc *os.Process
	renderedDockerFile []byte
}

func blockUntilSocket(timeout int) error {
	for i := 0; i<timeout; i++ {
		_, err := os.Stat(sockPath)
		if os.IsExist(err) {
			return nil
		}
		time.Sleep(1*time.Second)
	}
	return fmt.Errorf("reached timeout", )
}

func startPodman(sockpath string) (string, *os.Process, error) {

	pathstr := fmt.Sprintf("unix://%s", path.Clean(sockpath))

	args := []string{
		"system",
		"service",
		"--timeout",
		"0",
		pathstr,
	}
	cmd := exec.Command("podman", args...)

	err := cmd.Start()

	if err != nil {
		return "", nil, err
	}

	return pathstr, cmd.Process, nil
}

func NewDockerStack(config *DockerStackConfig) (*DockerStack, error) {
	renderedBuildScript, err := utils.RenderTemplate(buildtemplates.BuildTemplate, config)

	if err != nil {
		return nil, fmt.Errorf("failed to render dockerfile: $v", err)
	}

	dockerFile, err := utils.RenderTemplate(buildtemplates.DockerTemplate, config)
	if err != nil {
		return nil, fmt.Errorf("Failed to render build script %v", err)
	}

	ctx := context.Background()

	apiurl, proc, err := startPodman(sockPath)

	blockUntilSocket(10)

	if (err != nil) {
		return nil, fmt.Errorf("failed to start podman daemon: %v", err)
	}

	os.Setenv("DOCKER_HOST", apiurl)
	os.Setenv("DOCKER_API_VERSION", "1.40")
	cli, err := bindings.NewConnection(ctx, apiurl)

	if err != nil {
		return nil, fmt.Errorf("failed to create docker api client: %v", err)
	}

	stack := &DockerStack{
		config:	config,
		renderedBuildScript: renderedBuildScript,
		ctx: cli,
		statePath: path.Join(path.Clean(config.StatePath), ".localstack"),
		podmanProc: proc,
		renderedDockerFile: dockerFile,
	}

	return stack, nil
}

func (s *DockerStack) Shutdown() error {
	err := s.podmanProc.Signal(syscall.SIGTERM)

	if (err != nil) {
		return fmt.Errorf("failed to signal podman process: %v", err)
	}

	state, err := s.podmanProc.Wait()

	if (err != nil) {
		return fmt.Errorf("failed to wait for podman process: %v", err)
	}

	if (!state.Exited()) {
		return fmt.Errorf("podman process did not exit")
	}

	return nil
}

func (s *DockerStack) setupTmpDir() error {
	tar := new(archivex.TarFile)

	os.MkdirAll(path.Join(s.statePath, "build-ubuntu"), 0700)
	os.MkdirAll(path.Join(s.statePath, "mounts/script"), 0700)
	os.MkdirAll(path.Join(s.statePath, "mounts/keys"), 0700)
	os.MkdirAll(path.Join(s.statePath, "mounts/logs"), 0700)
	os.MkdirAll(path.Join(s.statePath, "mounts/release"), 0700)

	ibd, err := os.Create(path.Join(s.statePath, "build-ubuntu/install-build-deps.sh"))

	if err != nil {
		return fmt.Errorf("failed to create install-build-deps.sh: %v", err)
	}

	defer ibd.Close()

	ibd.WriteString(buildtemplates.ChromiumDeps)
	ibd.Sync()

	iad, err := os.Create(path.Join(s.statePath, "build-ubuntu/install-build-deps-android.sh"))

	if err != nil {
		return fmt.Errorf("failed to create install-build-deps-android.sh: %v", err)
	}

	defer iad.Close()

	iad.WriteString(buildtemplates.AndroidDeps)
	iad.Sync()

	df, err := os.Create(path.Join(s.statePath, "build-ubuntu/Dockerfile"))

	if err != nil {
		return fmt.Errorf("failed to write dockerfile")
	}
	df.Write(s.renderedDockerFile)
	df.Sync()

	defer df.Close()

	bs, err := os.Create(path.Join(s.statePath, "mounts/script/build.sh"))

	if err != nil {
		return fmt.Errorf("failed to write build script")
	}

	bs.Write(s.renderedBuildScript)
	bs.Sync()

	tar.Create(path.Join(s.statePath, "build-ubuntu.tar"))
	tar.AddAll(path.Join(s.statePath, "build-ubuntu"), true)
	tar.Close()
	return nil
}

func (s *DockerStack) containerExists() (bool, error) {
	filters := make(map[string][]string)

	filters["name"] = []string{containerName}


	containerList, err := containers.List(s.ctx, filters, nil, nil, nil, nil)

	if err != nil {
		return false, fmt.Errorf("failed to list containers: %v", err)
	}
	return len(containerList) == 0, nil
}

func (s *DockerStack) Build(force bool) error {
	args := []string{s.config.Device, strconv.FormatBool(force)}
	return s.containerExec(args, []string{}, false, true)
}

func (s *DockerStack) containerExec(args []string, env []string, async bool, stdin bool) error {
	log.Info("starting localstack build")

	exist, err := s.containerExists()

	if err != nil {
		return err
	}

	if exist {
		log.Info("Starting container")
		spec := specgen.NewSpecGenerator(imageTag, false)

		spec.Terminal = true
		spec.Name = containerName

		resp, err := containers.CreateWithSpec(s.ctx, spec)

		if err != nil {
			return fmt.Errorf("error creating container: %v", err)
		}


		err = containers.Start(s.ctx, resp.ID, nil)

		if err != nil {
			return fmt.Errorf("failed to start container: %v", err)
		}

		running := define.ContainerStateRunning

		_, err = containers.Wait(s.ctx, resp.ID, &running)

		if err != nil {
			return fmt.Errorf("failed to wait for container: %v", err)
		}
	} else {
		log.Info("Container already started")
	}
	return nil
}

func (s *DockerStack) Apply() error {
	//TODO: deploy docker envionment
	log.Info("deploying docker client")
	s.setupTmpDir()
	commonOpts := buildah.CommonBuildOptions{
		//TODO: volumes
	}

	imageBuildah := imagebuildah.BuildOptions{
		ContextDirectory: path.Join(s.statePath, "build-ubuntu"),
		PullPolicy: buildah.PullAlways,
		Quiet: false,
		Isolation: buildah.IsolationOCIRootless,
		Compression: archive.Gzip,
		Output: imageTag,
		Log: log.Infof,
		In: os.Stdin,
		Out: os.Stdout,
		ReportWriter: os.Stdout,
		CommonBuildOpts: &commonOpts,
	}

	buildoptions := entities.BuildOptions{
		imageBuildah,
	}

	containerfile := []string{path.Join(s.statePath, "build-ubuntu/Dockerfile")}

	_, err := images.Build(s.ctx, containerfile, buildoptions)

	if err != nil {
		return fmt.Errorf("failed to build image: %v", err)
	}
	return nil
}

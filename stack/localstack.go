package stack

import (
	"bufio"
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
	"github.com/containers/podman/v2/pkg/api/handlers"
	"github.com/containers/podman/v2/pkg/bindings"
	"github.com/containers/podman/v2/pkg/bindings/containers"
	"github.com/containers/podman/v2/pkg/bindings/images"
	"github.com/containers/podman/v2/pkg/bindings/volumes"
	"github.com/containers/podman/v2/pkg/domain/entities"
	"github.com/containers/podman/v2/pkg/specgen"
	"github.com/containers/storage/pkg/archive"
	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/docker/docker/api/types"
	"github.com/jhoonb/archivex"
	log "github.com/sirupsen/logrus"
	"github.io/gnu3ra/localstack/buildtemplates"
	"github.io/gnu3ra/localstack/utils"
)

const (
	imageTag = "localstack-build-image"
	sockPath = "/tmp/localstack.sock"
	containerName = "localstack-build"
	buildVolumeName = "localstack-build"
	keysVolumeName = "localstack-keys"
	scriptsVolumeName = "localstack-scripts"
	releaseVolumeName = "localstack-release"
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
	Uid					   string
	Gid					   string
}


type DockerStack struct {
	config *DockerStackConfig
	renderedBuildScript []byte
	buildScriptFileLocation string
	ctx context.Context
	statePath string
	scriptPath string
	keysPath string
	logsPath string
	buildPath string
	releasePath string
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
	statepath := path.Join(path.Clean(config.StatePath), ".localstack")
	stack := &DockerStack{
		config:	config,
		renderedBuildScript: renderedBuildScript,
		ctx: cli,
		statePath: statepath,
		podmanProc: proc,
		renderedDockerFile: dockerFile,
		scriptPath: path.Join(statepath, "mounts/script"),
		keysPath: path.Join(statepath, "mounts/keys"),
		logsPath: path.Join(statepath, "mounts/logs"),
		releasePath: path.Join(statepath, "mounts/release"),
		buildPath: path.Join(statepath, "build-ubuntu"),
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

	os.MkdirAll(s.buildPath, 0700)
	os.MkdirAll(s.scriptPath, 0700)
	os.MkdirAll(s.keysPath, 0700)
	os.MkdirAll(s.logsPath, 0700)
	os.MkdirAll(s.releasePath, 0700)

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

	bs, err := os.Create(path.Join(s.statePath, "build-ubuntu/build.sh"))

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
	args := []string{"/bin/bash", "/script/build.sh", s.config.Device, strconv.FormatBool(force)}
	return s.containerExec(args, []string{}, false, true)
}

func (s *DockerStack) setupVolume(name string) error {
	resp, err := volumes.Inspect(s.ctx, name)

	if err != nil || resp == nil {
		_, err := volumes.Create(s.ctx, entities.VolumeCreateOptions{
			Name: name,
		})

		if err != nil {
			return fmt.Errorf("Error, failed to create volume %v", err)
		}
	}

	return nil
}

func (s *DockerStack) setupVolumes() error {
	err := s.setupVolume(buildVolumeName)

	if err != nil {
		return err
	}

	err = s.setupVolume(scriptsVolumeName)

	if err != nil {
		return err
	}

	err = s.setupVolume(releaseVolumeName)

	if err != nil {
		return err
	}

	err = s.setupVolume(keysVolumeName)

	if err != nil {
		return err
	}

	return nil
}

func (s *DockerStack) containerExec(args []string, env []string, async bool, stdin bool) error {
	log.Info("starting localstack build")


	err := s.setupVolumes()

	if err != nil {
		return fmt.Errorf("failed to setup build volume: %v", err)
	}

	exist, err := s.containerExists()

	if err != nil {
		return err
	}

	if exist {
		var volume = false
		containers.Remove(s.ctx, containerName, nil, &volume)
	}

	log.Info("Starting container")
	spec := specgen.NewSpecGenerator(imageTag, false)

	releasemount := specs.Mount{
		Destination: "/release",
		Source: s.releasePath,
		Type: "bind",
	}

	buildvol := specgen.NamedVolume{
		Name: buildVolumeName,
		Dest: "/build",
	}

	keysvol := specgen.NamedVolume{
		Name: keysVolumeName,
		Dest: "/keys",
	}

	spec.Terminal = true
	spec.Name = containerName
	spec.Volumes = []*specgen.NamedVolume{&buildvol, &keysvol}
	spec.Mounts = []specs.Mount{releasemount}
	spec.Command = args

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

	opts := handlers.ExecCreateConfig{
		types.ExecConfig{
			AttachStderr: true,
			AttachStdout: true,
			AttachStdin: true,
			Env: env,
			Cmd: args,
		},
	}

	exec, err := containers.ExecCreate(s.ctx, containerName, &opts)

	if err != nil {
		return fmt.Errorf("ExecCreate failed: %v", err)
	}


	attachopts := define.AttachStreams{
		OutputStream: os.Stdout,
		ErrorStream: os.Stderr,
		InputStream: bufio.NewReader(os.Stdin),
		AttachOutput: true,
		AttachError: true,
		AttachInput: true,
	}

	err = containers.ExecStartAndAttach(s.ctx, exec, &attachopts)

	if err != nil {
		return fmt.Errorf("Failed to attach to container: %v", err)
	}


	stopped := define.ContainerStateStopped
	_, err = containers.Wait(s.ctx, resp.ID, &stopped)

	if err != nil {
		return fmt.Errorf("failed to wait for container: %v", err)
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
		PullPolicy: buildah.PullIfNewer,
		Quiet: false,
		Isolation: buildah.IsolationOCIRootless,
		Compression: archive.Gzip,
		Output: imageTag,
		Log: log.Infof,
		In: os.Stdin,
		Out: os.Stdout,
		ReportWriter: os.Stdout,
		CommonBuildOpts: &commonOpts,
		NoCache: false,
		Layers: true,
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

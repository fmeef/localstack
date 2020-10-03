package stack

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/jhoonb/archivex"
	log "github.com/sirupsen/logrus"
	"github.io/gnu3ra/localstack/buildtemplates"
	"github.io/gnu3ra/localstack/utils"
)

const (
	imageTag = "localstack-build-image"
	sockPath = "/tmp/localstack.sock"
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
	dockerClient *client.Client
	ctx context.Context
	statePath string
	podmanProc *os.Process
}

func startPodman(sockpath string) (string, *os.Process, error) {

	pathstr := fmt.Sprintf("unix://%s", path.Clean(sockpath))

	var procattr os.ProcAttr

	args := []string{
		"system",
		"service",
		"--timeout",
		"0",
		pathstr,
	}
	proc, err := os.StartProcess("podman", args, &procattr)

	if err != nil {
		return "", nil, err
	}

	return pathstr, proc, nil
}

func NewDockerStack(config *DockerStackConfig) (*DockerStack, error) {
	renderedBuildScript, err := utils.RenderTemplate(buildtemplates.BuildTemplate, config)

	if err != nil {
		return nil, fmt.Errorf("Failed to render build script %v", err)
	}

	ctx := context.Background()

	apiurl, proc, err := startPodman(sockPath)

	if (err != nil) {
		return nil, fmt.Errorf("failed to start podman daemon: %v", err)
	}

	os.Setenv("DOCKER_HOST", apiurl)
	os.Setenv("DOCKER_API_VERSION", "1.40")
	cli, err := client.NewEnvClient()

	if err != nil {
		return nil, fmt.Errorf("failed to create docker api client: %v", err)
	}

	stack := &DockerStack{
		config:	config,
		renderedBuildScript: renderedBuildScript,
		ctx: ctx,
		dockerClient: cli,
		statePath: path.Join(path.Clean(config.StatePath), ".localstack"),
		podmanProc: proc,
	}

	return stack, nil
}



func (s *DockerStack) setupTmpDir() error {
	tar := new(archivex.TarFile)

	os.MkdirAll(path.Join(s.statePath, "build-ubuntu"), 0700)
	os.MkdirAll(path.Join(s.statePath, "mounts/script"), 0700)
	os.MkdirAll(path.Join(s.statePath, "mounts/keys"), 0700)
	os.MkdirAll(path.Join(s.statePath, "mounts/logs"), 0700)
	os.MkdirAll(path.Join(s.statePath, "mounts/release"), 0700)

	dockerFile, err := utils.RenderTemplate(buildtemplates.DockerTemplate, s.config)

	if err != nil {
		return fmt.Errorf("failed to render docker template: %v", err)
	}

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
	df.Write(dockerFile)
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
	containers, err := s.dockerClient.ContainerList(s.ctx, types.ContainerListOptions{})

	if (err != nil) {
		return false, fmt.Errorf("error, failed to list contianers: %v", err)
	}

	for _, container := range containers {
		for _, label := range container.Labels {
			if (label == imageTag) {
				return true, nil
			}
		}
	}

	return false, nil
}

func (s *DockerStack) Build(force bool) error {
	args := []string{s.config.Device, strconv.FormatBool(force)}
	return s.containerExec(args, []string{}, false, true)
}

func (s *DockerStack) containerExec(args []string, env []string, async bool, stdin bool) error {
	log.Info("starting localstack build")

	opts := types.ExecConfig{
		Privileged: false,
		AttachStderr: true,
		AttachStdout: true,
		AttachStdin: stdin,
		Env: env,
		Cmd: args,
	}

	hijackedResponse, err := s.dockerClient.ContainerExecAttach(s.ctx, imageTag, opts)

	if (err != nil) {
		return fmt.Errorf("error, ContainerExecAttach failed: %v", err)
	}

	defer hijackedResponse.Close()


	if (stdin) {
		go io.Copy(hijackedResponse.Conn, os.Stdin)
	}

	if (!async || stdin) {
		io.Copy(os.Stdout, hijackedResponse.Reader)
	}

	return nil
}

func (s *DockerStack) Apply() error {
	//TODO: deploy docker envionment
	log.Info("deploying docker client")
	opt := types.ImageBuildOptions{
		SuppressOutput: false,
		Remove:         true,
		ForceRemove:    true,
		PullParent:     true,
		Dockerfile:     "build-ubuntu/Dockerfile",
		Tags:           []string{imageTag},
	}

	err := s.setupTmpDir()

	if err != nil {
		return err
	}

	buildCtx, err := os.Open(path.Join(s.statePath, "build-ubuntu.tar"))

	if err != nil {
		return fmt.Errorf("Failed to open docker build context")
	}

	defer buildCtx.Close()

	response, err := s.dockerClient.ImageBuild(s.ctx, buildCtx, opt)
	log.Info("deploying image")

	if err != nil {
		return fmt.Errorf("failed to run docker build %v", err)
	} else {
		log.Info("successfully created image")
	}

	defer response.Body.Close()

	type Stream struct {
		Stream string `json:"stream"`
	}

	d := json.NewDecoder(response.Body)

	for d.More() {
		var v Stream
		err = d.Decode(&v)
		if err == io.EOF {
			break
		}

		if err != nil {
			log.Fatal(err)
		}

		log.Info(v.Stream)
	}
	return nil
}

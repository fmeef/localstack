package stack

import (
	"context"
	"fmt"
	"path"
	"os"
	"encoding/json"
	"io"
	log "github.com/sirupsen/logrus"
	"github.com/docker/docker/client"
	"github.com/docker/docker/api/types"
	"github.io/gnu3ra/localstack/buildtemplates"
	"github.io/gnu3ra/localstack/utils"
	"github.com/jhoonb/archivex"
	"github.com/kyoh86/xdg"
)


type DockerStackConfig struct {
	Name                   string
	Device                 string
	Email                  string
	SSHKey                 string
	Version                string
	Schedule               string
	IgnoreVersionChecks    bool
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
}


func NewDockerStack(config *DockerStackConfig) (*DockerStack, error) {
	renderedBuildScript, err := utils.RenderTemplate(buildtemplates.BuildTemplate, config)

	if err != nil {
		return nil, fmt.Errorf("Failed to render build script %v", err)
	}

	ctx := context.Background()

	apiurl := fmt.Sprintf("unix:///%s/podman/podman.sock", xdg.RuntimeDir())
	os.Setenv("DOCKER_HOST", apiurl)
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

func (s *DockerStack) Apply() error {
	//TODO: deploy docker envionment
	opt := types.ImageBuildOptions{
		SuppressOutput: false,
		Remove:         true,
		ForceRemove:    true,
		PullParent:     true,
		Dockerfile:     "build-ubuntu/Dockerfile",
		Tags:           []string{"rattlesnake-build-image"},
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

	if err != nil {
		return fmt.Errorf("failed to run docker build %v", err)
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

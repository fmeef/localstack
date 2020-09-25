package stack

import (
	"fmt"
	"github.io/gnu3ra/localstack/buildtemplates"
	"github.io/gnu3ra/localstack/utils"
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
	Config *DockerStackConfig
	renderedBuildScript []byte
	BuildScriptFileLocation string
}


func NewDockerStack(config *DockerStackConfig) (*DockerStack, error) {
	renderedBuildScript, err := utils.RenderTemplate(buildtemplates.BuildTemplate, config)

	if err != nil {
		return nil, fmt.Errorf("Failed to render build script %v", err)
	}

	stack := &DockerStack{
		Config:	config,
		renderedBuildScript: renderedBuildScript,
	}

	return stack, nil
}

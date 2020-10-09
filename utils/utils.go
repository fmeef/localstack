package utils

import (
	"bytes"
	"io/ioutil"
	"text/template"
)

type CustomPatches []struct {
	Repo    string
	Patches []string
	Branch 	string
}

type CustomScripts []struct {
	Repo    string
	Scripts []string
	Branch 	string
}

type CustomPrebuilts []struct {
	Repo    string
	Modules []string
}

type CustomManifestRemotes []struct {
	Name     string
	Fetch    string
	Revision string
}

type CustomManifestProjects []struct {
	Path    string
	Name    string
	Remote  string
	Modules []string
}

func RenderTemplate(templateStr string, params interface{}) ([]byte, error) {
	templ, err := template.New("template").Delims("<%", "%>").Parse(templateStr)
	if err != nil {
		return nil, err
	}

	buffer := new(bytes.Buffer)

	if err = templ.Execute(buffer, params); err != nil {
		return nil, err
	}

	outputBytes, err := ioutil.ReadAll(buffer)
	if err != nil {
		return nil, err
	}
	return outputBytes, nil
}

package cli

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"github.io/gnu3ra/localstack/stack"
	"github.io/gnu3ra/localstack/utils"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/manifoldco/promptui"
	log "github.com/sirupsen/logrus"
	yaml "gopkg.in/yaml.v2"
)


const minimumChromiumVersion = 80

var deployCheck = func(cmd *cobra.Command, args []string) error {
		if viper.GetString("device") == "" {
			return errors.New("must specify device type")
		}

		if viper.GetString("device") == "marlin" || viper.GetString("device") == "sailfish" {
			log.Warnf("WARNING: marlin/sailfish devices are no longer receiving security updates and will likely be completely deprecated in the future")
		}
		if viper.GetString("chromium-version") != "" {
			chromiumVersionSplit := strings.Split(viper.GetString("chromium-version"), ".")
			if len(chromiumVersionSplit) != 4 {
				return errors.New("invalid chromium-version specified")
			}
			chromiumMajorNumber, err := strconv.Atoi(chromiumVersionSplit[0])
			if err != nil {
				return fmt.Errorf("unable to parse specified chromium-version: %v", err)
			}
			if chromiumMajorNumber < minimumChromiumVersion {
				return fmt.Errorf("pinned chromium-version must have major version of at least %v", minimumChromiumVersion)
			}
		}

		if device == "list" {
			fmt.Printf("Valid devices are: %v\n", supportDevicesOutput)
			os.Exit(0)
		}
		for _, device := range supportedDevicesCodename {
			if device == viper.GetString("device") {
				return nil
			}
		}
		return fmt.Errorf("must specify a supported device: %v", strings.Join(supportedDevicesCodename, ", "))
	}

var name, region, email, device, sshKey, maxPrice, skipPrice, schedule string
var instanceType, instanceRegions, hostsFile, chromiumVersion string
var preventShutdown, encryptedKeys, saveConfig, attestationServer bool
var patches = &utils.CustomPatches{}
var scripts = &utils.CustomScripts{}
var prebuilts = &utils.CustomPrebuilts{}
var manifestRemotes = &utils.CustomManifestRemotes{}
var manifestProjects = &utils.CustomManifestProjects{}
var trustedRepoBase = "https://github.com/gnu3ra/localstack"
var supportedDevicesFriendly = []string{"Pixel", "Pixel XL", "Pixel 2", "Pixel 2 XL", "Pixel 3", "Pixel 3 XL", "Pixel 3a", "Pixel 3a XL"}
var supportedDevicesCodename = []string{"sailfish", "marlin", "walleye", "taimen", "blueline", "crosshatch", "sargo", "bonito"}


var supportDevicesOutput string

func init() {
	rootCmd.AddCommand(deployCmd)

	for i, d := range supportedDevicesCodename {
		supportDevicesOutput += fmt.Sprintf("%v (%v)", d, supportedDevicesFriendly[i])
		if i < len(supportedDevicesCodename)-1 {
			supportDevicesOutput += ", "
		}
	}

	flags := deployCmd.Flags()
	flags.StringVarP(&device, "device", "d", "",
		"device you want to build for (e.g. crosshatch): to list supported devices use '-d list'")
	viper.BindPFlag("device", flags.Lookup("device"))

	flags.StringVar(&chromiumVersion, "chromium-version", "",
		"specify the version of Chromium you want (e.g. 80.0.3971.4) to pin to. if not specified, the latest stable "+
			"version of Chromium is used.")
	viper.BindPFlag("chromium-version", flags.Lookup("chromium-version"))

	flags.BoolVar(&saveConfig, "save-config", false, "allows you to save all passed CLI flags to config file")
}


var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy or update the AWS infrastructure used for building RattlesnakeOS",
	Args: deployCheck,
	Run: func(cmd *cobra.Command, args []string) {
		viper.UnmarshalKey("custom-patches", patches)
		viper.UnmarshalKey("custom-scripts", scripts)
		viper.UnmarshalKey("custom-prebuilts", prebuilts)
		viper.UnmarshalKey("custom-manifest-remotes", manifestRemotes)
		viper.UnmarshalKey("custom-manifest-projects", manifestProjects)

		c := viper.AllSettings()
		bs, err := yaml.Marshal(c)
		if err != nil {
			log.Fatalf("unable to marshal config to YAML: %v", err)
		}
		log.Println("Current settings:")
		fmt.Println(string(bs))

		if saveConfig {
			log.Printf("These settings will be saved to config file %v.", configFileFullPath)
		}

		for _, r := range *patches {
			if !strings.Contains(strings.ToLower(r.Repo), trustedRepoBase) {
				log.Warnf("You are using an untrusted repository (%v) for patches - this is risky unless you own the repository", r.Repo)
			}
		}

		for _, r := range *scripts {
			if !strings.Contains(strings.ToLower(r.Repo), trustedRepoBase) {
				log.Warnf("You are using an untrusted repository (%v) for scripts - this is risky unless you own the repository", r.Repo)
			}
		}

		for _, r := range *prebuilts {
			if !strings.Contains(strings.ToLower(r.Repo), trustedRepoBase) {
				log.Warnf("You are using an untrusted repository (%v) for prebuilts - this is risky unless you own the repository", r.Repo)
			}
		}

		prompt := promptui.Prompt{
			Label:     "Do you want to continue ",
			IsConfirm: true,
		}
		_, err = prompt.Run()
		if err != nil {
			log.Fatalf("Exiting %v", err)
		}

		s, err := stack.NewDockerStack(&stack.DockerStackConfig{
			Name:                   viper.GetString("name"),
			Device:                 viper.GetString("device"),
			Email:                  viper.GetString("email"),
			SSHKey:                 viper.GetString("ssh-key"),
			Schedule:               viper.GetString("schedule"),
			ChromiumVersion:        viper.GetString("chromium-version"),
			HostsFile:              viper.GetString("hosts-file"),
			CustomPatches:          patches,
			CustomScripts:          scripts,
			CustomPrebuilts:        prebuilts,
			CustomManifestRemotes:  manifestRemotes,
			CustomManifestProjects: manifestProjects,
			Version:                version,
			EnableAttestation:      viper.GetBool("attestation-server"),
			StatePath:              viper.GetString("statepath"),
			NumProc:                viper.GetInt("nproc"),
		})
		if err != nil {
			log.Fatal(err)
		}
		if err := s.Apply(); err != nil {
			log.Fatal(err)
		}

		if err = s.Shutdown(); err != nil {
			log.Fatalf("failed to shutdown: %v", err)
		}

		if saveConfig {
			log.Printf("Saved settings to config file %v.", configFileFullPath)
			err := viper.WriteConfigAs(configFileFullPath)
			if err != nil {
				log.Fatalf("Failed to write config file %v", configFileFullPath)
			}
		}
	},
}

package cli

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"
	"github.io/gnu3ra/localstack/stack"
	"github.com/spf13/viper"
)

var forceBuild bool

func init() {
	rootCmd.AddCommand(buildCmd)

	flags := buildCmd.Flags()

	flags.BoolVarP(&forceBuild, "force", "f", false, "skip version check and force a complete rebuild")
}



var buildCmd = &cobra.Command{
	Use: "build",
	Short: "Launched a one-shot build of localstack.",
	Args: func(cmd *cobra.Command, args []string) error {
		err := deployCheck(cmd, args)
		if (err != nil) {
			return fmt.Errorf("error: stack is not deployed: %v", err)
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		viper.UnmarshalKey("custom-patches", patches)
		viper.UnmarshalKey("custom-scripts", scripts)
		viper.UnmarshalKey("custom-prebuilts", prebuilts)
		viper.UnmarshalKey("custom-manifest-remotes", manifestRemotes)
		viper.UnmarshalKey("custom-manifest-projects", manifestProjects)
		c, err := stack.NewDockerStack(&stack.DockerStackConfig{
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

		if (err != nil) {
			log.Fatal(err)
			return
		}

		err = c.Build(forceBuild)

		if (err != nil) {
			log.Fatal(err)
		}
	},
}

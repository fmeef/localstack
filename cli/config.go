package cli

import (
	"errors"
	"fmt"
	"github.com/fatih/color"
	"github.com/manifoldco/promptui"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"math/rand"
	"os"
	"runtime"
	"strconv"
	"time"
)

func init() {
	rootCmd.AddCommand(configCmd)
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Setup config file for localstack",
	Run: func(cmd *cobra.Command, args []string) {
		color.Cyan(fmt.Sprintln("Device is the device codename (e.g. sailfish). Supported devices:", supportDevicesOutput))
		validate := func(input string) error {
			if len(input) < 1 {
				return errors.New("Device name is too short")
			}
			found := false
			for _, d := range supportedDevicesCodename {
				if input == d {
					found = true
					break
				}
			}
			if !found {
				return errors.New("Invalid device")
			}
			return nil
		}
		devicePrompt := promptui.Prompt{
			Label:    "Device ",
			Default:  viper.GetString("device"),
			Validate: validate,
		}
		result, err := devicePrompt.Run()
		if err != nil {
			log.Fatalf("Prompt failed %v\n", err)
		}
		viper.Set("device", result)

		defaultLocal := "AWS"

		validate = func(input string) error {
			if input != "local" && input != "AWS" {
				return errors.New("valid types are local or AWS only")
			}
			return nil
		}

		localPrompt := promptui.Prompt{
			Label:    "Local or aws?",
			Validate: validate,
			Default:  defaultLocal,
		}

		result, err = localPrompt.Run()

		if err != nil {
			log.Fatalf("Prompt failed %v\n", err)
		}

		color.Cyan(fmt.Sprintln("Path to store stateful files for local stack"))
		validate = func(input string) error {
			fileInfo, err := os.Stat(input)
			if err != nil {
				return err
			}

			if !fileInfo.IsDir() {
				return fmt.Errorf("error: path is not a directory")
			}
			return nil
		}

		dirPrompt := promptui.Prompt{
			Label:    "State path ",
			Validate: validate,
			Default:  "/home/.rattlesnake",
		}

		result, err = dirPrompt.Run()

		if err != nil {
			log.Fatalf("Prompt failed %v\n", err)
		}

		viper.Set("statepath", result)

		color.Cyan(fmt.Sprintln("number of cpus to use for build (too many can result in OOM condition"))

		validate = func(input string) error {
			_, err = strconv.ParseInt(input, 10, 64)
			if err != nil {
				return fmt.Errorf("enter an integer")
			}
			return nil
		}

		nprocPrompt := promptui.Prompt{
			Label:    "Number of processors ",
			Validate: validate,
			Default:  strconv.Itoa(runtime.NumCPU()),
		}

		result, err = nprocPrompt.Run()

		if err != nil {
			log.Fatalf("Prompt failed: %v", err)
		}

		viper.Set("nproc", result)

		defaultKeypairName := "rattlesnakeos"
		if viper.GetString("ssh-key") != "" {
			defaultKeypairName = viper.GetString("ssh-key")
		}
		color.Cyan(fmt.Sprintln("SSH keypair name is the name of your EC2 keypair that was imported into AWS."))
		validate = func(input string) error {
			if len(input) < 1 {
				return errors.New("SSH keypair name is too short")
			}
			return nil
		}
		keypairPrompt := promptui.Prompt{
			Label:    "SSH Keypair Name ",
			Default:  defaultKeypairName,
			Validate: validate,
		}
		result, err = keypairPrompt.Run()
		if err != nil {
			log.Fatalf("Prompt failed %v\n", err)
		}
		viper.Set("ssh-key", result)

		err = viper.WriteConfigAs(configFileFullPath)
		if err != nil {
			log.WithError(err).Fatalf("failed to write config file %s", configFileFullPath)
		}

		log.Infof("rattlesnakeos-stack config file has been written to %v", configFileFullPath)
	},
}

func randomString(strlen int) string {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, strlen)
	for i := range result {
		result[i] = chars[r.Intn(len(chars))]
	}
	return string(result)
}

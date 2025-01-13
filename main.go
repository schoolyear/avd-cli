package main

import (
	"context"
	"fmt"
	"github.com/schoolyear/avd-cli/bakedin"
	"github.com/schoolyear/avd-cli/commands"
	"github.com/schoolyear/avd-cli/lib"
	"github.com/urfave/cli/v2"
	"os"
	"time"
)

func main() {
	backgroundVersionCheckResult := make(chan string)
	go func() {
		version, _, err := lib.FetchLatestVersion(context.Background())
		if err == nil {
			backgroundVersionCheckResult <- version
		}
	}()

	app := &cli.App{
		Name:  "avdcli",
		Usage: "manage your AVD deployment",
		Description: `This tool helps you manage your exam-ready images.
Visit https://avd.schoolyear.com for more information on how to use this tool.`,
		Version: bakedin.Version,
		Suggest: true,
		Commands: cli.Commands{
			{
				Name:  "image",
				Usage: "manage images",
				Subcommands: cli.Commands{
					commands.ImageNewCommand,
					commands.ImagePackage,
				},
			},
			{
				Name:  "package",
				Usage: "manage image building packages",
				Subcommands: cli.Commands{
					commands.PackageDeployCommand,
				},
			},
			commands.UpdateCommand,
		},
		EnableBashCompletion: true,
		Authors: []*cli.Author{
			{
				Name:  "Schoolyear",
				Email: "support@schoolyear.com",
			},
		},
		Copyright: "Schoolyear",
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Println("Error:", err.Error())
		os.Exit(1)
	}

	select {
	case version := <-backgroundVersionCheckResult:
		if version != bakedin.Version {
			fmt.Printf("\n\nThere is a new version available (%s -> %s). Run \"avdcli update\" to update.\n", bakedin.Version, version)
		}
	case <-time.After(2 * time.Second):
	}
}

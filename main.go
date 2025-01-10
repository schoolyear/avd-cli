package main

import (
	"fmt"
	"github.com/schoolyear/avd-cli/commands"
	"github.com/urfave/cli/v2"
	"os"
)

var (
	Version = "v0.0.0"
)

func main() {
	app := &cli.App{
		Name:  "avdcli",
		Usage: "manage your AVD deployment",
		Description: `This tool helps you manage your exam-ready images.
Visit https://avd.schoolyear.com for more information on how to use this tool.`,
		Version: Version,
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
}

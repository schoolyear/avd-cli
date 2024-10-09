package main

import (
	"fmt"
	"github.com/schoolyear/avd-cli/commands"
	"github.com/urfave/cli/v2"
	"os"
	"time"
)

var (
	Version = "v0.0.0"
)

func main() {
	app := &cli.App{
		Name:    "avd-cli",
		Usage:   "managed you AVD deployment",
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
		Flags:                []cli.Flag{},
		EnableBashCompletion: true,
		Compiled:             time.Time{},
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

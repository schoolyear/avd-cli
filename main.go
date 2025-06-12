package main

import (
	"context"
	"fmt"
	"github.com/schoolyear/avd-cli/commands"
	"github.com/schoolyear/avd-cli/lib"
	"github.com/schoolyear/avd-cli/static"
	"github.com/urfave/cli/v2"
	"os"
	"sync/atomic"
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

	ctx := context.WithValue(context.Background(), static.CtxUpdatedKey, &atomic.Bool{})
	defer func() {
		updatedKey := ctx.Value(static.CtxUpdatedKey).(*atomic.Bool)
		if updatedKey.Load() {
			return
		}

		select {
		case version := <-backgroundVersionCheckResult:
			if version != static.Version {
				fmt.Printf("\n\nThere is a new version available (%s -> %s). Run \"avdcli update\" to download & install.\n", static.Version, version)
			}
		case <-time.After(2 * time.Second):
		}
	}()

	app := &cli.App{
		Name:  "avdcli",
		Usage: "manage your AVD deployment",
		Description: `This tool helps you manage your exam-ready images.
Visit https://avd.schoolyear.com for more information on how to use this tool.`,
		Version: static.Version,
		Suggest: true,
		Commands: cli.Commands{
			{
				Name:  "layer",
				Usage: "manage image layers",
				Subcommands: cli.Commands{
					commands.LayerNewCommand,
				},
			},
			{
				Name:  "bundle",
				Usage: "manage layer bundles",
				Subcommands: cli.Commands{
					commands.BundleLayersCommand,
					commands.BundleAutoDeployCommand,
				},
			},
			{
				Name:  "image",
				Usage: "manage images (used for v1 images)",
				Subcommands: cli.Commands{
					commands.ImageNewCommand,
					commands.ImagePackageCommand,
				},
			},
			{
				Name:  "package",
				Usage: "manage image building packages (used for v1 images)",
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

	if err := app.RunContext(ctx, os.Args); err != nil {
		fmt.Println("Error:", err.Error())
		os.Exit(1)
	}
}

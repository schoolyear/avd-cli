package commands

import (
	"fmt"
	"github.com/friendsofgo/errors"
	"github.com/schoolyear/avd-cli/embeddedfiles"
	"github.com/schoolyear/avd-cli/lib"
	"github.com/urfave/cli/v2"
	"path/filepath"
)

var LayerNewCommand = &cli.Command{
	Name:  "new",
	Usage: "create a new image layer folder structure",
	Flags: []cli.Flag{
		&cli.PathFlag{
			Name:     "output",
			Required: true,
			Usage:    "Path in which the new image layer folder should be created",
			Aliases:  []string{"o"},
		},
	},
	Action: func(c *cli.Context) error {
		targetPath := c.Path("output")

		if err := lib.EnsureEmptyDirectory(targetPath, false); err != nil {
			return errors.Wrap(err, "failed to create target directory")
		}

		absTargetPath, err := filepath.Abs(targetPath)
		if err != nil {
			return errors.Wrapf(err, "failed to convert target path to absolute path")
		}

		if err := lib.CopyDirectory(embeddedfiles.V2ImageTemplate, embeddedfiles.V2ImageTemplateBasePath, targetPath); err != nil {
			return errors.Wrap(err, "failed to copy image layer template directory")
		}

		fmt.Println("created new image layer folder at", absTargetPath)

		return nil
	},
}

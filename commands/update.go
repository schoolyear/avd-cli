package commands

import (
	"context"
	"fmt"
	"github.com/c-bata/go-prompt"
	"github.com/friendsofgo/errors"
	"github.com/schollz/progressbar/v3"
	"github.com/schoolyear/avd-cli/bakedin"
	"github.com/schoolyear/avd-cli/lib"
	"github.com/urfave/cli/v2"
	"golang.org/x/mod/semver"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
)

const CtxUpdatedKey = "avdcliupdated"

var UpdateCommand = &cli.Command{
	Name:  "update",
	Usage: "update your local avdcli",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "yes",
			Usage:   "Automatic yes to prompts; assume \"yes\" as answer to all prompts and run non-interactively.",
			Aliases: []string{"y"},
		},
		&cli.BoolFlag{
			Name:  "downgrade",
			Usage: "Force update to a lower version",
		},
	},
	Action: func(c *cli.Context) error {
		yesFlag := c.Bool("yes")
		downgradeFlag := c.Bool("downgrade")

		execPath, err := os.Executable()
		if err != nil {
			return errors.Wrap(err, "could not get the path of the current executable")
		}

		tmpFile := execPath + ".tmp"

		fmt.Println("Checking latest version...")
		latestVersion, latestDownloadURL, err := lib.FetchLatestVersion(c.Context)
		if err != nil {
			return errors.Wrap(err, "failed to fetch latest version")
		}

		fmt.Printf("Current: \t%s\nLatest: \t%s\n", c.App.Version, latestVersion)
		if latestVersion == c.App.Version {
			fmt.Println("You are on the latest version already!")
			return nil
		}

		if semver.IsValid(bakedin.Version) && semver.IsValid(latestVersion) && semver.Compare(bakedin.Version, latestVersion) == 1 {
			if downgradeFlag {
				fmt.Println("Note: this is a downgrade")
			} else {
				return errors.New("The latest public version is older than your current version. Use -downgrade to install the latest public version anyway")
			}
		}

		fmt.Printf("Download from:\t%s\n", latestDownloadURL)
		fmt.Printf("Download to:\t%s\n", tmpFile)
		fmt.Printf("Install to:\t%s\n", execPath)

		if !yesFlag {
			selected := strings.ToLower(prompt.Input(
				"Do you want to download & install the update (yes/no): ",
				lib.PromptNoCompletions(),
				lib.PromptOptionCtrlCExit(),
			))
			if selected != "yes" && selected != "y" {
				fmt.Println(`update canceled. You must enter "yes" or "y" to confirm.`)
				return nil
			}
		}

		if err := downloadUpdate(c.Context, latestDownloadURL, execPath, tmpFile); err != nil {
			return errors.Wrap(err, "failed to perform update")
		}
		fmt.Println("Update complete!")

		updatedKey := c.Context.Value(CtxUpdatedKey).(*atomic.Bool)
		updatedKey.Store(true)

		return nil
	},
}

func downloadUpdate(ctx context.Context, downloadUrl, targetPath, tmpFilePath string) error {
	originalFileInfo, err := os.Stat(targetPath)
	if err != nil {
		return errors.Wrap(err, "failed to get the file permissions of the current executable")
	}

	tmpFile, err := os.OpenFile(tmpFilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, originalFileInfo.Mode().Perm())
	if err != nil {
		return errors.Wrapf(err, "failed to create temp file in %s", tmpFilePath)
	}
	defer func() {
		tmpFile.Close()
		os.Remove(tmpFilePath)
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadUrl, nil)
	if err != nil {
		return errors.Wrap(err, "failed to create download request")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "failed to request download")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download latest version: %s", resp.Status)
	}

	downloadProgress := progressbar.DefaultBytes(
		resp.ContentLength,
		"Downloading",
	)

	_, err = io.Copy(io.MultiWriter(tmpFile, downloadProgress), resp.Body)
	if err != nil {
		return errors.Wrap(err, "failed to download and write update to disk")
	}

	err = os.Rename(tmpFile.Name(), targetPath)
	if err != nil {
		return errors.Wrap(err, "failed to replace current version with newly downloaded version")
	}

	return nil
}

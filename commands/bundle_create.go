package commands

import (
	"archive/zip"
	"encoding/json"
	stdErr "errors"
	"fmt"
	"github.com/friendsofgo/errors"
	"github.com/schoolyear/avd-cli/embeddedfiles"
	"github.com/schoolyear/avd-cli/lib"
	avdimagetypes "github.com/schoolyear/avd-image-types"
	"github.com/urfave/cli/v2"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var BundleLayersCommand = &cli.Command{
	Name:  "layers",
	Usage: "Bundle layers together",
	Flags: []cli.Flag{
		&cli.StringSliceFlag{
			Name:      "layer",
			Usage:     "Paths to the image layers",
			Required:  true,
			TakesFile: true,
			Aliases:   []string{"l"},
		},
		&cli.StringFlag{
			Name:      "bundle-output",
			Usage:     "Path to which the bundle output should be written",
			TakesFile: true,
			Aliases:   []string{"o"},
			Value:     "bundle.zip",
		},
	},
	Action: func(c *cli.Context) error {
		layerPaths := c.StringSlice("layer")
		bundleOutput := c.String("bundle-output")

		fmt.Println("Validating layers:")

		layers := make([]avdimagetypes.V2LayerProperties, 0, len(layerPaths))
		names := map[string]struct{}{}
		allValid := true
		for i, layerPath := range layerPaths {
			fmt.Printf("\t- Layer %d: %s: ", i+1, layerPath)

			properties, err := validateLayerPath(layerPath)
			if err != nil {
				allValid = false
				fmt.Printf("[Invalid]: \n\t\t%s\n", strings.ReplaceAll(err.Error(), "\n", "\n\t\t"))
				continue
			}

			if _, exists := names[properties.Name]; exists {
				allValid = false
				fmt.Printf("[Invalid]: layer name must be unique. %s already exists\n", properties.Name)
				continue
			}

			fmt.Printf("[Valid]: %s\n", properties.Name)

			for _, filename := range []string{"install.ps1", "on_sessionhost_setup.ps1", "on_user_login.admin.ps1", "on_user_login.user.ps1"} {
				filePath := filepath.Join(layerPath, filename)
				var status string
				_, err := os.Stat(filePath)
				if err == nil {
					status = "[Found]"
				} else if os.IsNotExist(err) {
					status = "[Not Found, but not required]"
				} else {
					status = fmt.Sprintf("[Error]: %s", err)
					allValid = false
				}
				fmt.Printf("\t\t%s: %s\n", filename, status)
			}

			layers = append(layers, *properties)
		}

		if allValid {
			fmt.Println("All layers are valid")
		} else {
			return errors.New("all layers must be valid to continue")
		}

		fmt.Println("")
		if err := copyLayersIntoBundle(layers, layerPaths, bundleOutput); err != nil {
			return errors.Wrap(err, "failed to copy layers into the bundle file")
		}

		fmt.Println("")
		showBaseImage(layers)

		return nil

		// decide which base image should be used
		// - default
		// - on conflict -> warning

		// copy all folders into a zip
		// add execute.ps1 script

		// combine all property files (json list of property file contents) -> write to json file
	},
}

func validateLayerPath(layerPath string) (*avdimagetypes.V2LayerProperties, error) {
	// check if dir exists
	fileInfo, err := os.Stat(layerPath)
	if err != nil {
		return nil, fmt.Errorf("layer path %s does not exist: %w", layerPath, err)
	}
	if !fileInfo.IsDir() {
		return nil, fmt.Errorf("layer path %s is not a directory", layerPath)
	}

	propertiesJson, _, err := lib.ReadJSONOrJSON5AsJSON(os.DirFS(layerPath), "properties")
	if err != nil {
		return nil, errors.Wrap(err, "failed to read properties file")
	}

	validationResult, err := avdimagetypes.ValidateV2Properties(propertiesJson)
	if err != nil {
		return nil, errors.Wrap(err, "failed to validate properties file")
	}

	if !validationResult.Valid() {
		resultErrors := validationResult.Errors()
		validationErrors := make([]error, len(resultErrors))
		for i, validationError := range resultErrors {
			validationErrors[i] = errors.New(validationError.String())
		}

		return nil, errors.Wrap(stdErr.Join(validationErrors...), "invalid properties file")
	}

	var properties avdimagetypes.V2LayerProperties
	if err := json.Unmarshal(propertiesJson, &properties); err != nil {
		return nil, errors.Wrap(err, "failed to parse properties file")
	}

	return &properties, nil
}

var defaultBaseImage = avdimagetypes.V2LayerPropertiesBaseImage{
	PlatformImage: &avdimagetypes.PlatformImage{
		Type:      avdimagetypes.PlatformImageTypePlatformImage,
		Publisher: "microsoftwindowsdesktop",
		Offer:     "windows-10",
		Sku:       "win10-22h2-avd-g2",
		Version:   "latest",
	},
}

func showBaseImage(layers []avdimagetypes.V2LayerProperties) {
	// find layers with base image definition
	var layerIdsWithBaseImageDefinition []int
	for i, layer := range layers {
		if layer.BaseImage != nil {
			layerIdsWithBaseImageDefinition = append(layerIdsWithBaseImageDefinition, i)
		}
	}

	switch len(layerIdsWithBaseImageDefinition) {
	case 0:
		fmt.Printf("No layer explicitly defines a base iamge, so we recommending building the image using this base image: \n\t%s\n", baseImageToString(defaultBaseImage))
	case 1:
		layer := layers[layerIdsWithBaseImageDefinition[0]]
		fmt.Printf("The layer %s defines the following base-image: \n%s\n", layer.Name, baseImageToString(*layer.BaseImage))
	default:
		fmt.Println("Multiple layers define a base image")
		for _, layerId := range layerIdsWithBaseImageDefinition {
			layer := layers[layerId]
			fmt.Printf("\t- Layer %s\n: %s", layer.Name, baseImageToString(*layer.BaseImage))
		}
		fmt.Println("You have to decide which one to use")
	}
}

func baseImageToString(image avdimagetypes.V2LayerPropertiesBaseImage) string {
	switch {
	case image.PlatformImage != nil:
		return fmt.Sprintf("Platform Image: %s/%s/%s:%s",
			image.PlatformImage.Publisher,
			image.PlatformImage.Offer,
			image.PlatformImage.Sku,
			image.PlatformImage.Version)
	case image.ManagedImage != nil:
		return fmt.Sprintf("Managed Image: %s", image.ManagedImage.ImageID)
	case image.SharedImageVersion != nil:
		return fmt.Sprintf("Shared Image Version: %s", image.SharedImageVersion.ImageVersionID)
	default:
		panic("unknown base image type")
	}
}

func copyLayersIntoBundle(layers []avdimagetypes.V2LayerProperties, layerPaths []string, targetPath string) error {
	fmt.Println("Creating the bundle file:")

	bundleFile, err := os.Create(targetPath)
	if err != nil {
		return errors.Wrap(err, "failed to create bundle zip file")
	}
	defer bundleFile.Close()

	zipWriter := zip.NewWriter(bundleFile)
	defer zipWriter.Close()

	fmt.Printf("\t- Adding %s...", embeddedfiles.V2ExecuteScriptFilename)
	executeFile, err := zipWriter.Create(embeddedfiles.V2ExecuteScriptFilename)
	if err != nil {
		return errors.Wrap(err, "failed to create execute script in the bundle")
	}
	if _, err := executeFile.Write(embeddedfiles.V2ExecuteScript); err != nil {
		return errors.Wrap(err, "failed to write execute script to the bundle")
	}
	fmt.Printf("[Done]\n")

	for i, layerPath := range layerPaths {
		layerName := layers[i].Name

		fmt.Printf("\t- Copying layer %s...", layerName)
		if err := copyLayerToBundle(zipWriter, layerName, layerPath); err != nil {
			return errors.Wrapf(err, "failed to copy layer %s to the bundle zip file", layerName)
		}
		fmt.Printf("[Done]\n")
	}

	fmt.Printf("Saved the bundle to: %s\n", targetPath)

	return nil
}

func copyLayerToBundle(zipFile *zip.Writer, layerName string, layerPath string) error {
	layerPathFs := os.DirFS(layerPath)

	// copied from zip.AddFS but with a modification to add the files to a subdirectory
	return fs.WalkDir(layerPathFs, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if name == "." {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !d.IsDir() && !info.Mode().IsRegular() {
			return errors.New("zip: cannot add non-regular file")
		}
		h, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		h.Name = filepath.Join(layerName, name) // here we add the name of the directory as a prefix
		if d.IsDir() {
			h.Name += "/"
		}
		h.Method = zip.Deflate
		fw, err := zipFile.CreateHeader(h)
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		f, err := layerPathFs.Open(name)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(fw, f)
		return err
	})
}

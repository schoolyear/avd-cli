package commands

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"github.com/friendsofgo/errors"
	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/schoolyear/avd-cli/embeddedfiles"
	"github.com/schoolyear/avd-cli/lib"
	"github.com/schoolyear/avd-cli/schema"
	"github.com/schoolyear/avd-cli/static"
	avdimagetypes "github.com/schoolyear/avd-image-types"
	"github.com/urfave/cli/v2"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const layerPropertiesFilename = "properties" // (either json or json5)

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
		&cli.PathFlag{
			Name:      "parameters",
			Usage:     "Path to a JSON file to pre-fill parameters",
			TakesFile: true,
			Aliases:   []string{"p"},
		},
		&cli.BoolFlag{
			Name:    "noninteractive",
			Usage:   "Set if you want to use non-interactive mode. Command will fail if interactivity is required.",
			Aliases: []string{"ni"},
		},
		&cli.PathFlag{
			Name:      "bundle-archive",
			Usage:     "Path where the bundle archive will be created",
			TakesFile: true,
			Aliases:   []string{"o"},
			Value:     "bundle.zip",
		},
		&cli.PathFlag{
			Name:      "bundle-properties",
			Usage:     "Path to which the bundle properties output should be written. Skipped if not set.",
			TakesFile: true,
			Aliases:   []string{"b"},
		},
	},
	Action: func(c *cli.Context) error {
		layerPaths := c.StringSlice("layer")
		parameterFile := c.Path("parameters")
		noninteractive := c.Bool("noninteractive")
		bundleOutput := c.Path("bundle-archive")
		bundleProperties := c.Path("bundle-properties")

		layers, err := validateLayers(layerPaths)
		if err != nil {
			return errors.Wrap(err, "failed to load and validate layers")
		}

		fmt.Println()
		fmt.Println("Resolving parameters")
		buildParameters, err := resolveLayerParameters(layers, parameterFile, noninteractive)
		if err != nil {
			return errors.Wrap(err, "failed to resolve parameters")
		}

		fmt.Println("")
		if err := copyLayersIntoBundle(layers, bundleOutput); err != nil {
			return errors.Wrap(err, "failed to copy layers into the bundle file")
		}

		fmt.Println("")
		showBaseImage(layers)

		fmt.Println()

		if bundleProperties == "" {
			fmt.Println("Not creating the bundle properties file. Set -bundle-properties (-p) to write it to disk.")
		} else {
			layerProperties := make([]avdimagetypes.V2LayerProperties, len(layers))
			for i, layer := range layers {
				layerProperties[i] = *layer.properties
			}
			if err := writeBundleProperties(layerProperties, buildParameters, bundleProperties); err != nil {
				return errors.Wrap(err, "failed to write bundle properties file")
			}
		}

		return nil
	},
}

type layerToBundle struct {
	originalPathName string // the original string used to reference this layer. may not be an actual path
	path             string
	fs               fs.FS
}

type validatedLayer struct {
	layerToBundle
	properties *avdimagetypes.V2LayerProperties
}

func validateLayers(layerPaths []string) ([]validatedLayer, error) {
	layersToBundle := make([]layerToBundle, 0, len(layerPaths)+1)

	layersToBundle = append(layersToBundle, layerToBundle{
		originalPathName: "default layer (built-in)",
		path:             embeddedfiles.V2DefaultLayerBasePath,
		fs:               embeddedfiles.V2DefaultLayer,
	})

	for _, path := range layerPaths {
		dirPath := filepath.Dir(path)
		dirFS := os.DirFS(dirPath)
		layersToBundle = append(layersToBundle, layerToBundle{
			originalPathName: path,
			path:             filepath.Base(path),
			fs:               dirFS,
		})
	}

	fmt.Println("Validating layers:")

	layers := make([]validatedLayer, 0, len(layersToBundle))
	names := map[string]struct{}{}
	allValid := true
	for i, layerToBundle := range layersToBundle {
		fmt.Printf("\t- Layer %d: %s: ", i+1, layerToBundle.originalPathName)

		layer, err := validateLayer(layerToBundle)
		if err != nil {
			allValid = false
			fmt.Printf("[Invalid]: \n\t\t%s\n", strings.ReplaceAll(err.Error(), "\n", "\n\t\t"))
			continue
		}

		if _, exists := names[layer.properties.Name]; exists {
			allValid = false
			fmt.Printf("[Invalid]: layer name must be unique. %s already exists\n", layer.properties.Name)
			continue
		} else {
			names[layer.properties.Name] = struct{}{}
		}

		fmt.Printf("[Valid]: %s\n", layer.properties.Name)

		filesToCheck := []string{
			schema.V2InstallScriptFilename,
			schema.V2OnSessionHostSetupScriptFilename,
			schema.V2OnUserLoginAdminScriptFilename,
			schema.V2OnUserLoginUserScriptFilename,
		}
		for _, filename := range filesToCheck {
			filePath := filepath.Join(layerToBundle.path, filename)
			var status string
			_, err := fs.Stat(layerToBundle.fs, filePath)
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

		layers = append(layers, *layer)
	}

	if allValid {
		fmt.Println("All layers are valid")
	} else {
		return nil, errors.New("all layers must be valid to continue")
	}

	return layers, nil
}

func validateLayer(layer layerToBundle) (*validatedLayer, error) {
	// check if dir exists
	fileInfo, err := fs.Stat(layer.fs, layer.path)
	if err != nil {
		return nil, fmt.Errorf("layer path %s does not exist: %w", layer.path, err)
	}
	if !fileInfo.IsDir() {
		return nil, fmt.Errorf("layer path %s is not a directory", layer.path)
	}

	propertiesJson, _, err := lib.ReadJSONOrJSON5AsJSON(layer.fs, filepath.Join(layer.path, layerPropertiesFilename))
	if err != nil {
		return nil, errors.Wrap(err, "failed to read properties file")
	}

	if err := lib.ValidateAVDImageType(avdimagetypes.V2LayerPropertiesDefinition, propertiesJson); err != nil {
		return nil, errors.Wrap(err, "invalid properties file")
	}

	var properties *avdimagetypes.V2LayerProperties
	if err := json.Unmarshal(propertiesJson, &properties); err != nil {
		return nil, errors.Wrap(err, "failed to parse properties file")
	}

	return &validatedLayer{
		layerToBundle: layer,
		properties:    properties,
	}, nil
}

func resolveLayerParameters(layers []validatedLayer, parameterFilePath string, noninteractive bool) (map[string]map[string]avdimagetypes.BuildParameterValue, error) {
	var prefilledParams map[string]map[string]avdimagetypes.BuildParameterValue

	if parameterFilePath != "" {
		data, err := os.ReadFile(parameterFilePath)
		if err != nil {
			return nil, errors.Wrap(err, "failed to read parameters file")
		}

		if err := lib.ValidateAVDImageType(avdimagetypes.V2BuildParametersDefinition, data); err != nil {
			return nil, errors.Wrap(err, "failed to validate parameters file")
		}

		var paramFile avdimagetypes.V2BuildParameters
		if err := json.Unmarshal(data, &paramFile); err != nil {
			return nil, errors.Wrap(err, "failed to parse parameters file")
		}

		prefilledParams = paramFile.Layers
	}

	resolvedParameters := map[string]map[string]avdimagetypes.BuildParameterValue{}
	fmt.Println("Resolving build parameters per layer:")
	for _, layer := range layers {
		fmt.Printf("\t- %s: %d parameter(s) to resolve\n", layer.properties.Name, len(layer.properties.BuildParameters))

		for paramName, param := range layer.properties.BuildParameters {
			fmt.Printf("\t\t- %s: ", paramName)

			prefilled := getPrefilledParameter(prefilledParams, layer.properties.Name, paramName)
			var value string
			if prefilled != nil {
				if len(param.Enum) > 0 {
					if err := validation.Validate(prefilled.Value, validation.In(param.Enum)); err != nil {
						return nil, errors.Wrapf(err, "invalid prefilled parameter: %s/%s", layer.properties.Name, paramName)
					}
				}
				fmt.Printf("[PREFILLED]: %s\n", paramName)
				value = prefilled.Value
			} else if noninteractive {
				return nil, fmt.Errorf("missing build parameter %s/%s, but running in noninteractive mode", layer.properties.Name, paramName)
			} else if len(param.Enum) > 0 {
				fmt.Println(param.Description)

				options := make([]string, len(param.Enum))
				var defaultIdx *int
				for i, option := range param.Enum {
					options[i] = option
					if param.Default == option {
						idx := i
						defaultIdx = &idx
					}
				}
				idx, err := lib.PromptEnum("Pick one", options, "\t\t\t", defaultIdx)
				if err != nil {
					return nil, err
				}
				value = param.Enum[idx]
			} else {
				fmt.Println(param.Description)

				var defaultValue *string
				if param.Default != "" {
					defaultValue = &param.Default
				}
				input, err := lib.PromptUserInput("\t\t\tEnter a value: ", defaultValue)
				if err != nil {
					return nil, err
				}

				if err := lib.ValidateAVDImageType(avdimagetypes.V2BuildParameterValueDefinition, []byte(`"`+input+`"`)); err != nil {
					return nil, errors.Wrap(err, "invalid build parameter value")
				}
				value = input
			}

			buildParamValue := avdimagetypes.BuildParameterValue{
				Value: value,
			}
			if layerParams, ok := resolvedParameters[layer.properties.Name]; ok {
				layerParams[paramName] = buildParamValue
			} else {
				layerParams := map[string]avdimagetypes.BuildParameterValue{
					paramName: buildParamValue,
				}
				resolvedParameters[layer.properties.Name] = layerParams
			}
		}
	}

	return resolvedParameters, nil
}

func getPrefilledParameter(prefilledParams map[string]map[string]avdimagetypes.BuildParameterValue, layerName, paramName string) *avdimagetypes.BuildParameterValue {
	layer, ok := prefilledParams[layerName]
	if !ok {
		return nil
	}
	param, ok := layer[paramName]
	if !ok {
		return nil
	}
	return &param
}

var defaultBaseImage = &avdimagetypes.V2LayerPropertiesBaseImage{
	PlatformImage: &avdimagetypes.PlatformImage{
		Type:      avdimagetypes.PlatformImageTypePlatformImage,
		Publisher: "microsoftwindowsdesktop",
		Offer:     "windows-10",
		Sku:       "win10-22h2-avd-g2",
		Version:   "latest",
	},
}

func showBaseImage(layers []validatedLayer) {
	// find layers with base image definition
	var layerIdsWithBaseImageDefinition []int
	for i, layer := range layers {
		if layer.properties.BaseImage != nil {
			layerIdsWithBaseImageDefinition = append(layerIdsWithBaseImageDefinition, i)
		}
	}

	switch len(layerIdsWithBaseImageDefinition) {
	case 0:
		fmt.Printf("No layer explicitly defines a base image, so we recommending building the image using this base image: \n\t%s\n", baseImageToString(defaultBaseImage))
	case 1:
		layer := layers[layerIdsWithBaseImageDefinition[0]]
		fmt.Printf("The layer %s defines the following base-image: \n\t%s\n", layer.properties.Name, baseImageToString(layer.properties.BaseImage))
	default:
		fmt.Println("Multiple layers define a base image")
		for _, layerId := range layerIdsWithBaseImageDefinition {
			layer := layers[layerId]
			fmt.Printf("\t- Layer %s\n: %s", layer.properties.Name, baseImageToString(layer.properties.BaseImage))
		}
		fmt.Println("You have to decide which one to use")
	}
}

func baseImageToString(image *avdimagetypes.V2LayerPropertiesBaseImage) string {
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

func copyLayersIntoBundle(layers []validatedLayer, targetPath string) error {
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
	fmt.Printf("[DONE]\n")

	for i, layer := range layers {
		layerName := fmt.Sprintf("%03d-%s", i+1, layers[i].properties.Name)

		fmt.Printf("\t- Copying layer %s...", layerName)
		if err := copyLayerToBundle(zipWriter, layerName, layer.fs, layer.path); err != nil {
			return errors.Wrapf(err, "failed to copy layer %s to the bundle zip file", layerName)
		}
		fmt.Printf("[DONE]\n")
	}

	fmt.Printf("Saved the bundle to: %s\n", targetPath)

	return nil
}

func copyLayerToBundle(zipFile *zip.Writer, layerName string, sourceFS fs.FS, sourcePath string) error {
	source, err := fs.Sub(sourceFS, sourcePath)
	if err != nil {
		return errors.Wrap(err, "failed to open the source directory")
	}

	// copied from zip.AddFS but with a modification to add the files to a subdirectory
	return fs.WalkDir(source, ".", func(name string, d fs.DirEntry, err error) error {
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
		f, err := source.Open(name)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(fw, f)
		return err
	})
}

func writeBundleProperties(layers []avdimagetypes.V2LayerProperties, buildParameters map[string]map[string]avdimagetypes.BuildParameterValue, path string) error {
	fmt.Printf("Creating the bundle properties file...")

	bundle := avdimagetypes.V2BundleProperties{
		Version:         avdimagetypes.V2BundlePropertiesVersionV2,
		CliVersion:      static.Version,
		Layers:          layers,
		BuildParameters: buildParameters,
	}

	propFile, err := os.Create(path)
	if err != nil {
		return errors.Wrap(err, "failed to create file")
	}
	defer propFile.Close()

	encoder := json.NewEncoder(propFile)
	encoder.SetIndent("", "\t")
	if err := encoder.Encode(bundle); err != nil {
		return errors.Wrap(err, "failed to write to file")
	}
	fmt.Printf("[DONE]: %s\n", path)
	return nil
}

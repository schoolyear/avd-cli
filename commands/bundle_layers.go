package commands

import (
	"archive/zip"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/fatih/color"
	"github.com/friendsofgo/errors"
	"github.com/go-resty/resty/v2"
	"github.com/schollz/progressbar/v3"
	"github.com/schoolyear/avd-cli/embeddedfiles"
	"github.com/schoolyear/avd-cli/lib"
	"github.com/schoolyear/avd-cli/schema"
	"github.com/schoolyear/avd-cli/static"
	avdimagetypes "github.com/schoolyear/avd-image-types"
	"github.com/urfave/cli/v2"
)

const layerPropertiesFilename = "properties" // (either json or json5)

var BundleLayersCommand = &cli.Command{
	Name:  "layers",
	Usage: "Bundle layers together",
	Flags: []cli.Flag{
		&cli.StringSliceFlag{
			Name:      "layer",
			Usage:     "Paths to local image layers or download layers on-demand using @community:name[@versiontag]",
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
		&cli.PathFlag{
			Name:  "community-cache",
			Usage: "Path to a folder in which the community cache can be stored",
			Value: "~/.avdcli/community-cache",
		},
		&cli.BoolFlag{
			Name:  "ignore-keyring",
			Usage: "Set if you want to ignore any existing tokens in your local keyring and force reauthentication",
		},
		&cli.BoolFlag{
			Name:  "no-keyring-cache",
			Usage: "Set if you do not want to store tokens in your local keyring for later use",
		},
	},
	Action: func(c *cli.Context) error {
		layerPaths := c.StringSlice("layer")
		parameterFile := c.Path("parameters")
		noninteractive := c.Bool("noninteractive")
		bundleOutput := c.Path("bundle-archive")
		bundleProperties := c.Path("bundle-properties")
		communityCachePath := c.Path("community-cache")
		ignoreKeyring := c.Bool("ignore-keyring")
		noKeyringCache := c.Bool("no-keyring-cache")

		// resolve ~ for community cache folder
		if strings.HasPrefix(communityCachePath, "~") {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return errors.Wrap(err, "failed to get home directory")
			}
			communityCachePath = filepath.Join(homeDir, communityCachePath[1:])
		}

		client := resty.New().
			SetTimeout(10 * time.Second).
			SetRetryCount(2).
			SetRetryWaitTime(1 * time.Second)

		parsedLayerPaths, hasCommunityLayers := parseLayerPaths(layerPaths)

		var githubToken string
		if hasCommunityLayers {
			token, err := lib.GithubDeviceFlow(client, static.GithubAppClientId, !ignoreKeyring, !noKeyringCache, "To download community layers, you must authenticate with GitHub")
			if err != nil {
				return errors.Wrap(err, "failed to authenticate with GitHub")
			}
			githubToken = token
		}

		layersToBundle, err := resolveLayersToBundle(client, parsedLayerPaths, communityCachePath, githubToken)
		if err != nil {
			return errors.Wrap(err, "failed to resolve layers to bundle")
		}

		fmt.Println()

		layers, err := validateLayers(layersToBundle)
		if err != nil {
			return errors.Wrap(err, "failed to load and validate layers")
		}

		fmt.Println()
		resolvedParameters, err := resolveLayerParameters(layers, parameterFile, noninteractive)
		if err != nil {
			return errors.Wrap(err, "failed to resolve parameters")
		}

		fmt.Println("")
		buildParameters := avdimagetypes.V2BuildParameters{
			Version: avdimagetypes.V2BuildParametersVersionV2,
			Layers:  resolvedParameters,
		}
		if err := copyLayersIntoBundle(layers, buildParameters, bundleOutput); err != nil {
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
			if err := writeBundleProperties(layerProperties, resolvedParameters, bundleProperties); err != nil {
				return errors.Wrap(err, "failed to write bundle properties file")
			}
		}

		color.HiGreen("Successfully created bundle!")

		return nil
	},
}

type parsedLayerPath struct {
	originalValue string
	community     *communityLayerPath
}

type communityLayerPath struct {
	name string
	ref  string
}

func parseLayerPaths(layerPaths []string) (layers []parsedLayerPath, hasCommunityLayers bool) {
	layers = make([]parsedLayerPath, len(layerPaths))
	for i, layerPath := range layerPaths {
		layerPath = filepath.Clean(layerPath)

		layer := parsedLayerPath{
			originalValue: layerPath,
		}

		_, reference, isCommunityLayer := strings.Cut(layerPath, "@community:")
		if isCommunityLayer {
			name, version, ok := strings.Cut(reference, "@")
			if !ok {
				version = "main"
			}

			layer.community = &communityLayerPath{
				name: name,
				ref:  version,
			}
			hasCommunityLayers = true
		}

		layers[i] = layer
	}

	return layers, hasCommunityLayers
}

func resolveLayersToBundle(client *resty.Client, parsedLayerPaths []parsedLayerPath, communityCachePath string, githubToken string) ([]layerToBundle, error) {
	layersToBundle := make([]layerToBundle, 0, len(parsedLayerPaths)+1)

	layersToBundle = append(layersToBundle, layerToBundle{
		originalPathName: "default layer (built-in)",
		path:             embeddedfiles.V2DefaultLayerBasePath,
		fs:               embeddedfiles.V2DefaultLayer,
	})

	fmt.Println("Resolving layers to bundle:")
	for _, layerPath := range parsedLayerPaths {
		fmt.Printf("    - %-60s ", layerPath.originalValue+":")

		var layer layerToBundle

		if layerPath.community == nil {
			dirPath := filepath.Dir(layerPath.originalValue)
			dirFS := os.DirFS(dirPath)
			layer = layerToBundle{
				originalPathName: layerPath.originalValue,
				path:             filepath.Base(layerPath.originalValue),
				fs:               dirFS,
			}

			fmt.Println("LOCAL")
		} else {
			fmt.Printf("scanning repository...")

			localPath, err := downloadLayerFromGithub(client, *layerPath.community, communityCachePath, githubToken)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to download layer %s from the community repository", layerPath.originalValue)
			}

			dirPath := filepath.Dir(localPath)
			dirFS := os.DirFS(dirPath)
			layer = layerToBundle{
				originalPathName: layerPath.originalValue,
				path:             filepath.Base(localPath),
				fs:               dirFS,
			}

			fmt.Println()
		}

		layersToBundle = append(layersToBundle, layer)
	}

	return layersToBundle, nil
}

func downloadLayerFromGithub(client *resty.Client, layer communityLayerPath, cachePath, githubToken string) (path string, err error) {
	// find layer tree sha
	items, err := lib.GithubListContents(client, githubToken, static.GithubImageCommunityOwner, static.GithubImageCommunityRepo, static.GithubImageCommunityLayerPath, &layer.ref)
	if err != nil {
		return "", errors.Wrap(err, "failed to list available community layers from Github")
	}

	var treeSha string
	for _, item := range items {
		if item.Type == "dir" && item.Name == layer.name {
			treeSha = item.SHA
			break
		}
	}
	if treeSha == "" {
		return "", errors.Errorf("%s not found in the community (ref:%s)", layer.name, layer.ref)
	}

	layerTree, err := lib.GithubListTree(client, githubToken, static.GithubImageCommunityOwner, static.GithubImageCommunityRepo, treeSha, true)
	if err != nil {
		return "", errors.Wrap(err, "failed to list files in community layer")
	}

	if layerTree.Truncated {
		return "", errors.Errorf("%s@%s has too many files to download from Github automatically", layer.name, layer.ref)
	}

	var filesToDownload []fileToDownload
	totalSize := int64(0)
	for _, tree := range layerTree.Tree {
		if tree.Type == "blob" && tree.Size != nil && tree.URL != nil {
			var fileMode os.FileMode
			switch tree.Mode {
			case "100644": // file
				fileMode = 0644
			case "100755": // executable
				fileMode = 0755
			default:
				return "", errors.Errorf("unsupported file mode %s", tree.Mode)
			}

			filesToDownload = append(filesToDownload, fileToDownload{
				path: tree.Path,
				mode: fileMode,
				sha:  tree.SHA,
				size: *tree.Size,
				url:  *tree.URL,
			})
			totalSize += *tree.Size
		}
	}

	humanSize := humanize.Bytes(uint64(totalSize))
	fmt.Printf("%d files (%s)\n", len(filesToDownload), humanSize)

	progress := progressbar.NewOptions(int(totalSize),
		progressbar.OptionSetElapsedTime(true),
		progressbar.OptionSetPredictTime(true),
		progressbar.OptionShowBytes(true),
	)

	const description = "        Downloading"
	progress.Describe(description)

	layerCachePath := filepath.Join(cachePath, treeSha)
	cacheHitCount := 0
	for _, file := range filesToDownload {
		cacheHit, err := downloadGithubFile(client, layerCachePath, file, progress)
		if err != nil {
			return "", errors.Wrapf(err, "failed to download %s from GitHub", file.url)
		}
		if cacheHit {
			_ = progress.Add64(file.size)
			cacheHitCount++
		}
	}

	_ = progress.Finish()
	fmt.Printf("\n        Cache hits: %d/%d\n", cacheHitCount, len(filesToDownload))

	return layerCachePath, nil
}

type fileToDownload struct {
	path string
	mode os.FileMode
	sha  string
	size int64
	url  string
}

func downloadGithubFile(client *resty.Client, layerCachePath string, file fileToDownload, bar *progressbar.ProgressBar) (cacheHit bool, err error) {
	targetPath := filepath.Join(layerCachePath, file.path)
	targetDir := filepath.Dir(targetPath)

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return false, errors.Wrapf(err, "failed to create directory %s", targetDir)
	}

	f, err := os.OpenFile(targetPath, os.O_CREATE|os.O_RDWR, file.mode)
	if err != nil {
		return false, errors.Wrapf(err, "failed to create file %s", targetPath)
	}
	defer f.Close()

	hash := sha1.New()

	// the Git SHA hash includes a header
	hash.Write([]byte("blob "))
	hash.Write([]byte(strconv.Itoa(int(file.size))))
	hash.Write([]byte{0})

	if _, err := io.Copy(hash, f); err != nil {
		return false, errors.Wrapf(err, "failed to calculate sha of file %s", targetPath)
	}
	existingSha := hash.Sum(nil)
	existingShaHex := hex.EncodeToString(existingSha)
	if existingShaHex == file.sha {
		return true, nil
	}
	if err := f.Truncate(0); err != nil {
		return false, errors.Wrap(err, "failed to truncate cached but outdated file")
	}
	if _, err := f.Seek(0, 0); err != nil {
		return false, errors.Wrap(err, "failed to seek back to start of outdated file")
	}

	res, err := client.R().
		SetDoNotParseResponse(true).
		SetHeader("Accept", "application/vnd.github.raw+json").
		Get(file.url)
	if err != nil {
		return false, errors.Wrapf(err, "failed to download %s from %s", file.path, file.url)
	}
	body := res.RawBody()
	defer body.Close()

	if res.StatusCode() != 200 {
		return false, errors.Wrapf(err, "failed to download (%d): %s", res.StatusCode(), res.String())
	}

	if _, err := io.Copy(f, io.TeeReader(body, bar)); err != nil {
		return false, errors.Wrap(err, "download failed")
	}

	return false, nil
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

func validateLayers(layersToBundle []layerToBundle) ([]validatedLayer, error) {
	fmt.Println("Validating layers:")

	layers := make([]validatedLayer, 0, len(layersToBundle))
	names := map[string]struct{}{}
	allValid := true
	for i, layerToBundle := range layersToBundle {
		fmt.Printf("    - Layer %d: %-60s ", i+1, layerToBundle.originalPathName+":")

		layer, err := validateLayer(layerToBundle)
		if err != nil {
			allValid = false
			color.HiRed("[Invalid]: \n        %s\n", strings.ReplaceAll(err.Error(), "\n", "\n        "))
			continue
		}

		if _, exists := names[layer.properties.Name]; exists {
			allValid = false
			color.HiRed("[Invalid]: layer name must be unique. %s already exists\n", layer.properties.Name)
			continue
		} else {
			names[layer.properties.Name] = struct{}{}
		}

		color.Green("[Valid]: %s\n", layer.properties.Name)

		filesToCheck := []string{
			schema.V2InstallScriptFilename,
			schema.V2OnSessionHostSetupScriptFilename,
			schema.V2OnUserLoginAdminScriptFilename,
			schema.V2OnUserLoginUserScriptFilename,
		}
		for _, filename := range filesToCheck {
			// use path.join instead of filepath.join, because fs.FS always expects a forward slash, independent of OS
			filePath := path.Join(layerToBundle.path, filename)
			var status string
			_, err := fs.Stat(layerToBundle.fs, filePath)
			if err == nil {
				status = color.GreenString("[Found]")
			} else if os.IsNotExist(err) {
				status = color.CyanString("[Not Found]")
			} else {
				status = color.HiRedString("[Error]: %s", err)
				allValid = false
			}
			fmt.Printf("        %-30s: %s\n", filename, status)
		}

		layers = append(layers, *layer)

		fmt.Println()
	}

	if allValid {
		color.HiGreen("All layers are valid")
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

	// use path.join instead of filepath.join, because fs.FS always expects a forward slash, independent of OS
	propertiesJson, _, err := lib.ReadJSONOrJSON5AsJSON(layer.fs, path.Join(layer.path, layerPropertiesFilename))
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
			return nil, errors.Wrap(err, "invalid parameters file")
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
		fmt.Printf("    - %s: %d parameter(s) to resolve\n", layer.properties.Name, len(layer.properties.BuildParameters))

		sortedParameterNames := make([]string, 0, len(layer.properties.BuildParameters))
		for paramName := range layer.properties.BuildParameters {
			sortedParameterNames = append(sortedParameterNames, paramName)
		}
		slices.Sort(sortedParameterNames)

		for _, paramName := range sortedParameterNames {
			param := layer.properties.BuildParameters[paramName]
			fmt.Printf("        - %s: ", paramName)

			prefilled := getPrefilledParameter(prefilledParams, layer.properties.Name, paramName)

			var value string
			if prefilled != nil {
				if len(param.Enum) > 0 {
					found := false
					for _, option := range param.Enum {
						if prefilled.Value == option {
							found = true
							break
						}
					}
					if !found {
						return nil, fmt.Errorf("invalid prefilled parameter %s/%s, expected one of (%s), got: %s", layer.properties.Name, paramName, strings.Join(param.Enum, ", "), prefilled.Value)
					}
				}
				fmt.Printf("[PREFILLED]: %s\n", prefilled.Value)
				value = prefilled.Value
			} else if noninteractive {
				return nil, fmt.Errorf("missing build parameter %s/%s, but running in noninteractive mode", layer.properties.Name, paramName)
			} else {
				var err error
				value, err = resolveLayerParameterInteractively(param)
				if err != nil {
					return nil, errors.Wrapf(err, "failed to resolve parameter %s/%s interactively", layer.properties.Name, paramName)
				}
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

func resolveLayerParameterInteractively(param avdimagetypes.LayerParameter) (value string, err error) {
	if len(param.Enum) > 0 {
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
		idx, err := lib.PromptEnum(color.YellowString("Pick one"), options, "            ", defaultIdx)
		if err != nil {
			return "", err
		}
		return param.Enum[idx], nil
	} else {
		fmt.Println(param.Description)

		var defaultValue *string
		if param.Default != "" {
			defaultValue = &param.Default
		}
		input, err := lib.PromptUserInput("            Enter a value: ", defaultValue)
		if err != nil {
			return "", err
		}

		if err := lib.ValidateAVDImageType(avdimagetypes.V2BuildParameterValueDefinition, []byte(`"`+input+`"`)); err != nil {
			return "", errors.Wrap(err, "invalid build parameter value")
		}

		return input, nil
	}
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
		fmt.Printf("No layer explicitly defines a base image, so we recommending building the image using this base image: \n    %s\n", baseImageToString(defaultBaseImage))
	case 1:
		layer := layers[layerIdsWithBaseImageDefinition[0]]
		fmt.Printf("The layer %s defines the following base-image: \n    %s\n", layer.properties.Name, baseImageToString(layer.properties.BaseImage))
	default:
		fmt.Println("Multiple layers define a base image")
		for _, layerId := range layerIdsWithBaseImageDefinition {
			layer := layers[layerId]
			fmt.Printf("    - Layer %s\n: %s", layer.properties.Name, baseImageToString(layer.properties.BaseImage))
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

func copyLayersIntoBundle(layers []validatedLayer, buildParams avdimagetypes.V2BuildParameters, targetPath string) error {
	fmt.Println("Creating the bundle file:")

	bundleFile, err := os.Create(targetPath)
	if err != nil {
		return errors.Wrap(err, "failed to create bundle zip file")
	}
	defer bundleFile.Close()

	zipWriter := zip.NewWriter(bundleFile)
	defer zipWriter.Close()

	fmt.Printf("    - Adding %s...", embeddedfiles.V2ExecuteScriptFilename)
	executeFile, err := zipWriter.Create(embeddedfiles.V2ExecuteScriptFilename)
	if err != nil {
		return errors.Wrap(err, "failed to create execute script in the bundle")
	}
	if _, err := executeFile.Write(embeddedfiles.V2ExecuteScript); err != nil {
		return errors.Wrap(err, "failed to write execute script to the bundle")
	}
	fmt.Printf("[DONE]\n")

	fmt.Printf("    - Adding %s...", schema.V2BuildParametersFilename)
	buildParamsData, err := json.MarshalIndent(buildParams, "", "    ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal build parameters to JSON")
	}

	buildParamsFile, err := zipWriter.Create(schema.V2BuildParametersFilename)
	if err != nil {
		return errors.Wrap(err, "failed to create the build parameters file in the bundle")
	}
	if _, err := buildParamsFile.Write(buildParamsData); err != nil {
		return errors.Wrap(err, "failed to write the build parameters file to the bundle")
	}
	fmt.Printf("[DONE]\n")

	for i, layer := range layers {
		layerName := fmt.Sprintf("%03d-%s", i+1, layers[i].properties.Name)

		fmt.Printf("    - Copying layer %s...", layerName)
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
	encoder.SetIndent("", "    ")
	if err := encoder.Encode(bundle); err != nil {
		return errors.Wrap(err, "failed to write to file")
	}
	fmt.Printf("[DONE]: %s\n", path)
	return nil
}

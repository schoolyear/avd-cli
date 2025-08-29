package commands

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"

	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/virtualmachineimagebuilder/armvirtualmachineimagebuilder"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"
	"github.com/friendsofgo/errors"
	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/joho/godotenv"
	"github.com/schollz/progressbar/v3"
	"github.com/schoolyear/avd-cli/lib"
	"github.com/schoolyear/avd-cli/schema"
	"github.com/urfave/cli/v2"
)

var PackageDeployCommand = &cli.Command{
	Name:  "deploy",
	Usage: "Deploy a package to Azure",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "name",
			Usage:    "Unique name for the Image Template in Azure",
			Required: true,
			Aliases:  []string{"n"},
		},
		&cli.StringFlag{
			Name:     "subscription",
			Required: true,
			Aliases:  []string{"s"},
		},
		&cli.StringFlag{
			Name:     "resource-group",
			Required: true,
			Aliases:  []string{"rg"},
		},
		&cli.PathFlag{
			Name:    "package",
			Value:   "./out",
			Usage:   "Path to the image building package",
			Aliases: []string{"p"},
		},
		&cli.PathFlag{
			Name:    "resources-uri",
			Usage:   "The URI path to which the resources archive can be uploaded. Required if the package contains a \"" + schema.SourceURIPlaceholder + "\" placeholder (almost always the case). E.g. \"https://<storageaccount>.blob.core.windows.net/<containername>\"",
			Aliases: []string{"r"},
		},
		&cli.StringFlag{
			Name:    "azure-tenant-id",
			Usage:   "Overwrite the default Azure Tenant ID",
			Aliases: []string{"atd"},
		},
		&cli.BoolFlag{
			Name:  "start",
			Usage: "Start image builder",
		},
		&cli.BoolFlag{
			Name:  "wait",
			Usage: "Wait for image builder to complete. Ignored if \"start\" flag is not set. This could take hours",
		},
		&cli.DurationFlag{
			Name:  "timeout",
			Usage: "Set after how much time the command should timeout. Especially useful in combination with \"-wait\". Valid time units are \"ns\", \"us\" (or \"µs\"), \"ms\", \"s\", \"m\", \"h\", which you can combine like this \"1h30m\"",
		},
		&cli.StringSliceFlag{
			Name:      "env",
			Usage:     "Paths to .env files to resolve package parameters from",
			Aliases:   []string{"e"},
			TakesFile: true,
		},
		&cli.BoolFlag{
			Name:    "resolve-interactively",
			Usage:   "Resolve missing parameters interactively",
			Aliases: []string{"i"},
			Value:   true,
		},
		&cli.StringFlag{
			Name:      "deployment-template-output",
			Usage:     "Path to save the resolved deployment template",
			Required:  true,
			Aliases:   []string{"dto"},
			TakesFile: true,
		},
		&cli.StringFlag{
			Name:  "parameters",
			Usage: "A coma separated list of parameter key=value pairs to be automatically resolved",
		},
	},
	Action: func(c *cli.Context) error {
		imageTemplateName := c.Path("name")
		subscription := c.Path("subscription")
		resourceGroup := c.Path("resource-group")
		packagePath := c.Path("package")
		resourcesURIString := c.Path("resources-uri")
		azureTenantID := c.String("azure-tenant-id")
		startImageBuilderFlag := c.Bool("start")
		waitForImageCompletion := c.Bool("wait")
		timeoutFlag := c.Duration("timeout")
		envFilePaths := c.StringSlice("env")
		resolveInteractively := c.Bool("resolve-interactively")
		deploymentTemplateOutput := c.Path("deployment-template-output")

		argParams, err := parseParametersFromArgument(c.String("parameters"))
		if err != nil {
			return errors.Wrap(err, "failed to parse parameters from argument")
		}

		ctx := context.Background()
		if timeoutFlag > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(context.Background(), timeoutFlag)
			defer cancel()
		}

		cwd, err := os.Getwd()
		if err != nil {
			return errors.Wrap(err, "failed to get current working directory")
		}
		fullPackagePath := filepath.Join(cwd, packagePath)

		packageFs := os.DirFS(packagePath)
		imageProperties, resourcesArchiveChecksum, err := scanPackagePath(packageFs, deploymentTemplateOutput, envFilePaths, argParams, resolveInteractively)
		if err != nil {
			return errors.Wrapf(err, "failed to scan image package directory %s", fullPackagePath)
		}

		if err := validation.Validate(imageProperties); err != nil {
			return errors.Wrap(err, "invalid image properties file")
		}

		var resourcesURI *storageAccountBlob
		if resourcesURIString != "" {
			var err error
			resourcesURI, err = parseResourcesURI(resourcesURIString)
			if err != nil {
				return errors.Wrap(err, "failed to parse resources-uri flag")
			}

			resourcesURI.Path = path.Join(resourcesURI.Path, fmt.Sprintf("%x.zip", resourcesArchiveChecksum))
		}

		if err := replaceSourceURIPlaceholder(imageProperties.ImageTemplate.V, resourcesURI); err != nil {
			return errors.Wrap(err, "failed to replace resources URI placeholder")
		}

		azCred, err := azidentity.NewDefaultAzureCredential(&azidentity.DefaultAzureCredentialOptions{
			TenantID: azureTenantID,
		})
		if err != nil {
			return errors.Wrap(err, "failed to get default Azure Credentials")
		}

		if resourcesURI != nil {
			fmt.Println("Uploading resources archive")
			alreadyUploaded, err := uploadResourcesArchive(ctx, azCred, resourcesURI, packageFs, resourcesArchiveName)
			if err != nil {
				return errors.Wrap(err, "failed to upload resources archive")
			}
			if alreadyUploaded {
				fmt.Println("Already uploaded")
			} else {
				fmt.Println("Uploaded")
			}
		}

		clientFactory, err := armvirtualmachineimagebuilder.NewClientFactory(subscription, azCred, nil)
		if err != nil {
			return errors.Wrap(err, "failed to initialize Azure SDK")
		}

		imageBuilderClient := clientFactory.NewVirtualMachineImageTemplatesClient()

		fmt.Println("Deploying image building template: " + imageTemplateName)
		imageTemplateResourceID, err := createImageTemplate(ctx, imageBuilderClient, resourceGroup, imageTemplateName, imageProperties)
		if err != nil {
			return err
		}
		fmt.Println("Image Template created: ", imageTemplateResourceID)

		if startImageBuilderFlag {
			fmt.Println("Starting image builder")
			if err := startImageBuilder(context.Background(), imageBuilderClient, resourceGroup, imageTemplateName, waitForImageCompletion); err != nil {
				return errors.Wrap(err, "failed to start image builder")
			}
		}

		return nil
	},
}

var errMalformedParams = errors.New("malformed parameters")

func parseParametersFromArgument(paramStr string) (map[string]string, error) {
	if paramStr == "" {
		return nil, nil
	}

	// split string on ','
	splitParams := strings.Split(paramStr, ",")
	if len(splitParams) < 1 {
		return nil, errMalformedParams
	}

	// split string on '='
	params := make(map[string]string, len(splitParams))
	for _, sp := range splitParams {
		keyValueParam := strings.Split(sp, "=")
		if len(keyValueParam) != 2 || len(keyValueParam[0]) == 0 || len(keyValueParam[1]) == 0 {
			return nil, errMalformedParams
		}

		params[keyValueParam[0]] = keyValueParam[1]
	}

	return params, nil
}

type storageAccountBlob struct {
	Service   string
	Container string
	Path      string
}

func (s storageAccountBlob) toURL() string {
	return fmt.Sprintf("%s/%s/%s", s.serviceURL(), s.Container, s.Path)
}

func (s storageAccountBlob) serviceURL() string {
	return fmt.Sprintf("https://%s", s.Service)
}

func parseResourcesURI(resourcesURI string) (*storageAccountBlob, error) {
	parsed, err := url.ParseRequestURI(resourcesURI)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse as an URI")
	}

	pathParts := strings.SplitN(strings.TrimPrefix(parsed.Path, "/"), "/", 2)
	var blobPath string
	switch len(pathParts) {
	case 1:
		if len(strings.Trim(pathParts[0], "/")) == 0 {
			return nil, fmt.Errorf("expected at least container name to be included in URI path")
		}
	case 2:
		blobPath = pathParts[1]
	default:
		panic("programming error: no more than 2 entries expected")
	}

	return &storageAccountBlob{
		Service:   parsed.Host,
		Container: pathParts[0],
		Path:      blobPath,
	}, nil
}

func scanPackagePath(packageFs fs.FS, deploymentTemplateOutputPath string, envFiles []string, argumentParameters map[string]string, resolveInteractively bool) (imageProperties *schema.ImageProperties, archiveSha256 []byte, err error) {
	// resolve parameters in the properties file
	propertiesFile, err := packageFs.Open(imagePropertiesFileWithExtension)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to open properties file")
	}
	defer propertiesFile.Close()

	propertiesFileContent, err := io.ReadAll(propertiesFile)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to read properties file")
	}

	var resolvedParams map[string]string
	paramsToResolve := schema.FindPlaceholdersInJSON(propertiesFileContent, schema.ParameterPlaceholder)
	if len(paramsToResolve) > 0 {
		fmt.Printf("Resolving %d package parameters\n", len(paramsToResolve))
		var err error
		resolvedParams, err = resolveParameters(envFiles, argumentParameters, paramsToResolve, resolveInteractively)
		if err != nil {
			return nil, nil, errors.Wrap(err, "failed to resolve parameters")
		}

		propertiesFileContent = schema.ReplacePlaceholders(propertiesFileContent, resolvedParams, schema.ParameterPlaceholder)
	}

	resolvedPropertiesFileContent, err := resolvePlaceholderProperties(propertiesFileContent)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to resolve placeholder properties")
	}

	if err := json.Unmarshal(resolvedPropertiesFileContent, &imageProperties); err != nil {
		return nil, nil, errors.Wrapf(err, "failed to parse image properties json:\n%s", resolvedPropertiesFileContent)
	}

	// resolve parameters in deployment template
	deploymentTemplateFile, err := packageFs.Open(deploymentTemplateFileWithExtension)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to open deployment template file")
	}
	defer deploymentTemplateFile.Close()

	originalDeploymentTemplateFileContents, err := io.ReadAll(deploymentTemplateFile)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to read deployment template file contents")
	}

	// first do a pass with previously resolved parameters from the properties file content
	// so we don't resolve them again
	resolvedDeploymentTemplateFileContents := schema.ReplacePlaceholders(originalDeploymentTemplateFileContents, resolvedParams, schema.ParameterPlaceholder)

	// search for unresolved parameters
	paramsToResolve = schema.FindPlaceholdersInJSON(resolvedDeploymentTemplateFileContents, schema.ParameterPlaceholder)
	if len(paramsToResolve) > 0 {
		fmt.Printf("Resolving %d deployment template parameters\n", len(paramsToResolve))
		resolvedParams, err := resolveParameters(envFiles, argumentParameters, paramsToResolve, resolveInteractively)
		if err != nil {
			return nil, nil, errors.Wrap(err, "failed to resolve parameters")
		}

		resolvedDeploymentTemplateFileContents = schema.ReplacePlaceholders(resolvedDeploymentTemplateFileContents, resolvedParams, schema.ParameterPlaceholder)
	}

	// Write the deployment template output file
	if err := os.WriteFile(deploymentTemplateOutputPath, resolvedDeploymentTemplateFileContents, 0644); err != nil {
		return nil, nil, errors.Wrap(err, "failed to write deployment template file to disk")
	}

	hash := sha256.New()
	resourcesFile, err := packageFs.Open(resourcesArchiveName)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to open resources archive")
	}
	defer resourcesFile.Close()

	resourcesFileStats, err := resourcesFile.Stat()
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to check for resources archive")
	}
	if resourcesFileStats.IsDir() {
		return nil, nil, errors.New("resources archive is expected to be a file, but it is a directory")
	}

	bar := progressbar.DefaultBytes(resourcesFileStats.Size(), "Calculating resources archive checksum")

	if _, err := io.Copy(io.MultiWriter(bar, hash), resourcesFile); err != nil {
		return nil, nil, errors.Wrap(err, "failed to calculate resources archive checksum")
	}

	return imageProperties, hash.Sum(nil), nil
}

func replaceSourceURIPlaceholder(imageTemplate armvirtualmachineimagebuilder.ImageTemplate, resourcesURI *storageAccountBlob) error {
	var fileCustomizersWithPlaceholders []*armvirtualmachineimagebuilder.ImageTemplateFileCustomizer
	if imageTemplate.Properties != nil {
		for _, step := range imageTemplate.Properties.Customize {
			customizer := step.GetImageTemplateCustomizer()
			if customizer.Type != nil && *customizer.Type == "File" {
				fileCustomizer := step.(*armvirtualmachineimagebuilder.ImageTemplateFileCustomizer)
				if fileCustomizer.SourceURI != nil && *fileCustomizer.SourceURI == schema.SourceURIPlaceholder {
					fileCustomizersWithPlaceholders = append(fileCustomizersWithPlaceholders, fileCustomizer)
				}
			}
		}
	}
	if len(fileCustomizersWithPlaceholders) > 0 {
		if resourcesURI == nil {
			return fmt.Errorf(`resource-uri flag required, since package contains a "%s" placeholder`, schema.SourceURIPlaceholder)
		}

		for _, customizer := range fileCustomizersWithPlaceholders {
			customizer.SourceURI = to.Ptr(resourcesURI.toURL())
		}
	}

	return nil
}

func resolveParameters(envFiles []string, argumentParameters map[string]string, params map[string]struct{}, resolveInteractively bool) (map[string]string, error) {
	env, err := godotenv.Read(envFiles...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read env files")
	}

	resolvedParams := make(map[string]string, len(params))
	var unresolvedParams []string
	for param := range params {
		// argument parameters take precedence
		// resolve parameter from arguments if exists
		argValue, ok := argumentParameters[param]
		if ok {
			resolvedParams[param] = argValue
			fmt.Printf("\t%s=%s\n", param, argValue)
			continue
		}

		// Then we check environment variables
		// if we can't find from environment aswell
		// we mark as unresolved to resolve interactively
		value, ok := env[param]
		if !ok {
			unresolvedParams = append(unresolvedParams, param)

			message := "UNRESOLVED"
			if resolveInteractively {
				message = "RESOLVE-INTERACTIVELY"
			}
			fmt.Printf("\t%s=>%s\n", param, message)
		} else {
			resolvedParams[param] = value
			fmt.Printf("\t%s=%s\n", param, value)
		}
	}

	if len(unresolvedParams) > 0 {
		if !resolveInteractively {
			return nil, errors.New("could not resolve all parameters, interactivity is disabled")
		}

		// make sure the params are always requested in specific order
		slices.Sort(unresolvedParams)

		fmt.Printf("Resolving %d parameter(s) interactively:\n", len(unresolvedParams))

		for i, param := range unresolvedParams {
			value, err := lib.PromptUserInput(fmt.Sprintf("(%d/%d) Enter value for %s (press Enter for empty): ", i+1, len(unresolvedParams), param), nil)
			if err != nil {
				return nil, errors.Wrap(err, "failed to prompt user for input")
			}

			resolvedParams[param] = value
		}
	}

	if len(resolvedParams) != len(params) {
		return nil, errors.New("could not resolve all parameters")
	}

	return resolvedParams, nil
}

func resolvePlaceholderProperties(imagePropsJSON []byte) ([]byte, error) {
	var imageProperties schema.ImageProperties
	if err := json.Unmarshal(imagePropsJSON, &imageProperties); err != nil {
		return nil, errors.Wrap(err, "failed to parse custom properties")
	}

	for {
		props := schema.FindPlaceholdersInJSON(imagePropsJSON, schema.PropertiesPlaceholder)
		if len(props) == 0 {
			break
		}

		mapping := make(map[string]string, len(props))
		for prop := range props {
			jsonValue, ok := imageProperties.PlaceholderProperties[prop]
			if !ok {
				return nil, fmt.Errorf("cannot resolve placeholder property %s", prop)
			}

			// if the value is not a string, it is some
			if !bytes.HasPrefix(jsonValue, []byte(`"`)) {
				var buf bytes.Buffer
				if err := json.Compact(&buf, jsonValue); err != nil {
					return nil, errors.Wrap(err, "failed to compact json-based custom property")
				}
				jsonValue = buf.Bytes()
			}

			escapedJSONValue, err := json.Marshal(string(jsonValue))
			if err != nil {
				panic("marshalling string shouldn't fail: " + err.Error())
			}

			mapping[prop] = string(escapedJSONValue[1 : len(escapedJSONValue)-1])
		}

		imagePropsJSON = schema.ReplacePlaceholders(imagePropsJSON, mapping, schema.PropertiesPlaceholder)
	}

	return imagePropsJSON, nil
}

func uploadResourcesArchive(ctx context.Context, azCreds azcore.TokenCredential, resourcesURI *storageAccountBlob, resourcesFs fs.FS, resourcesArchivePath string) (alreadyUploaded bool, err error) {
	azBlobClient, err := azblob.NewClient(resourcesURI.serviceURL(), azCreds, nil)
	if err != nil {
		return false, errors.Wrap(err, "failed to initialize Azure SDK")
	}

	_, err = azBlobClient.ServiceClient().NewContainerClient(resourcesURI.Container).NewBlobClient(resourcesURI.Path).GetProperties(ctx, nil)
	if err != nil && !bloberror.HasCode(err, bloberror.BlobNotFound) {
		return false, errors.Wrap(err, "failed to check if resources archive is already uploaded")
	} else if err == nil {
		return true, nil
	}

	resourcesFile, err := resourcesFs.Open(resourcesArchivePath)
	if err != nil {
		return false, errors.Wrap(err, "failed to open resources archive")
	}
	defer resourcesFile.Close()

	resourcesFileStat, err := resourcesFile.Stat()
	if err != nil {
		return false, errors.Wrap(err, "failed to check resource archive size")
	}

	bar := progressbar.DefaultBytes(resourcesFileStat.Size(), "Upload resources archive")
	defer bar.Exit()

	progressReader := progressbar.NewReader(resourcesFile, bar)
	defer progressReader.Close()

	_, err = azBlobClient.UploadStream(ctx, resourcesURI.Container, resourcesURI.Path, progressReader.Reader, nil)
	if err != nil {
		return false, errors.Wrap(err, "failed to upload")
	}

	bar.Finish()
	return false, nil
}

func createImageTemplate(ctx context.Context, imageTemplateClient *armvirtualmachineimagebuilder.VirtualMachineImageTemplatesClient, resourceGroup string, imageTemplateName string, imageProperties *schema.ImageProperties) (string, error) {
	bar := progressbar.Default(-1, "Creating Image Template resource")
	createTemplatePoller, err := imageTemplateClient.BeginCreateOrUpdate(
		ctx,
		resourceGroup,
		imageTemplateName,
		imageProperties.ImageTemplate.V,
		nil,
	)
	if err != nil {
		return "", errors.Wrap(err, "failed to create Image Template")
	}

	createTemplateRes, err := createTemplatePoller.PollUntilDone(context.Background(), nil)
	bar.Finish()
	if err != nil {
		return "", errors.Wrap(err, "failed to wait until Image Template is created")
	}

	return *createTemplateRes.ID, nil
}

func startImageBuilder(ctx context.Context, imageTemplateClient *armvirtualmachineimagebuilder.VirtualMachineImageTemplatesClient, resourceGroup, name string, wait bool) error {
	poller, err := imageTemplateClient.BeginRun(ctx, resourceGroup, name, nil)
	if err != nil {
		return errors.Wrap(err, "failed to call beginRun api")
	}

	if wait {
		fmt.Println("Started Image Builder...this may take up to a few hours to finish")
		_, err := poller.PollUntilDone(ctx, nil)
		if err != nil {
			return errors.Wrap(err, "failed to poll until image builder is finished")
		}
		fmt.Println("Image Builder finished. Check the Azure Portal")
	} else {
		fmt.Println("Started image builder. You can track the progress in the Azure Portal")
	}

	return nil
}

package commands

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	stdErr "errors"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/virtualmachineimagebuilder/armvirtualmachineimagebuilder/v2"
	"github.com/fatih/color"
	"github.com/friendsofgo/errors"
	"github.com/schoolyear/avd-cli/embeddedfiles"
	"github.com/schoolyear/avd-cli/lib"
	"github.com/schoolyear/avd-cli/schema"
	avdimagetypes "github.com/schoolyear/avd-image-types"
	"github.com/urfave/cli/v2"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var BundleAutoDeployCommand = &cli.Command{
	Name:  "autobuild",
	Usage: "Auto-build the bundle using Azure Image Template Builder",
	Flags: []cli.Flag{
		&cli.PathFlag{
			Name:    "bundle",
			Value:   "bundle.zip",
			Usage:   "Path to the bundle zip file",
			Aliases: []string{"b"},
		},
		&cli.StringFlag{
			Name:     "subscription-id",
			Usage:    "Azure subscription ID",
			Required: true,
			Aliases:  []string{"sid"},
		},
		&cli.StringFlag{
			Name:     "resource-group",
			Usage:    "Name of the Resource Group in which the Image Template Builder is created and in which the Image Gallery is stored.",
			Required: true,
			Aliases:  []string{"g"},
		},
		&cli.StringFlag{
			Name:  "layer-base-image",
			Usage: "Name of the layer that decides the base layer. You will be prompted when not set and multiple layers define a base image",
		},
		&cli.StringFlag{
			Name:     "image-gallery",
			Usage:    "Name of the shared image gallery",
			Required: true,
			Aliases:  []string{"sig"},
		},
		&cli.StringFlag{
			Name:    "image-definition",
			Usage:   "Name of the image definition in the shared image gallery. You will be prompted when not set",
			Aliases: []string{"i"},
		},
		&cli.StringFlag{
			Name:     "storage-account",
			Usage:    "Name of the Azure Storage Account to use to storage the bundle",
			Required: true,
			Aliases:  []string{"sa"},
		},
		&cli.StringFlag{
			Name:     "blob-container",
			Usage:    "Name of Azure Blob Container to use to store the bundle",
			Required: true,
			Aliases:  []string{"bc"},
		},
		&cli.StringFlag{
			Name:  "template-name",
			Usage: "Overwrite the name of the Image Template (defaults to [image-definition]-[timestamp]). Note that the operation will fail if this name is already taken.",
		},
		&cli.StringSliceFlag{
			Name:     "replication-regions",
			Usage:    "Names of the regions to replicate the image to",
			Required: true,
			Aliases:  []string{"reg"},
		},
		&cli.StringFlag{
			Name:    "builder-size",
			Usage:   "VM size to use for the builder",
			Value:   "Standard_D2s_v4",
			Aliases: []string{"size"},
		},
		&cli.StringFlag{
			Name:  "builder-vm-size",
			Usage: "VM size to use for the builder",
			Value: "Standard_D2s_v4",
		},
		&cli.UintFlag{
			Name:  "builder-disk-size",
			Usage: "VM size to use for the builder",
			Value: 127,
		},
		&cli.StringFlag{
			Name:     "managed-identity",
			Usage:    "Managed identity to use during image building",
			Required: true,
			Aliases:  []string{"mid"},
		},
		&cli.UintFlag{
			Name:  "build-timeout",
			Usage: "Build timeout in minutes",
			Value: 180,
		},
		&cli.BoolFlag{
			Name:  "start",
			Usage: "Automatically start the builder",
			Value: true,
		},
		&cli.BoolFlag{
			Name:  "exclude-from-latest",
			Usage: `Don't tag new image version as "latest"`,
			Value: false,
		},
		&cli.UintFlag{
			Name:  "replication-count",
			Usage: "Number of disk replications in each region. Azure recommends one for every 50 concurrently start students.",
			Value: 5,
		},
		&cli.BoolFlag{
			Name:  "optimize-image",
			Usage: "Enabled VM Boot optimization during image build (experimental)",
		},
		&cli.StringFlag{
			Name:     "template-location",
			Usage:    "Location to create the template in",
			Required: true,
		},
		&cli.PathFlag{
			Name:      "deployment-template",
			Usage:     "Filepath to which to write the deployment template",
			TakesFile: true,
			Value:     "deploy.json",
		},
		&cli.BoolFlag{
			Name:  "skip-deployment",
			Usage: "Skip the deployment of the Image Template. This allows you to tweak the Image Template deployment before manually deploying it.",
			Value: false,
		},
		&cli.PathFlag{
			Name:      "bundle-properties",
			Usage:     "Path to which the bundle properties output will be written.",
			TakesFile: true,
			Aliases:   []string{"p"},
			Value:     "bundle.json",
		},
	},
	Action: func(c *cli.Context) error {
		now := time.Now()
		nowFormatted := now.Format("2006-01-02_15-04-05")

		bundlePath := c.Path("bundle")
		subscriptionId := c.String("subscription-id")
		resourceGroup := c.String("resource-group")
		layerBaseImageName := c.String("layer-base-image")
		imageGallery := c.String("image-gallery")
		imageDefinition := c.String("image-definition")
		storageAccount := c.String("storage-account")
		blobContainer := c.String("blob-container")
		templateName := c.String("template-name")
		replicationRegions := c.StringSlice("replication-regions")
		builderVmSize := c.String("builder-vm-size")
		builderDiskSize := c.Uint("builder-disk-size")
		managedIdentity := c.String("managed-identity")
		buildTimeout := c.Uint("build-timeout")
		start := c.Bool("start")
		excludeFromLatest := c.Bool("exclude-from-latest")
		replicationCount := c.Uint("replication-count")
		optimizeImage := c.Bool("optimize-image")
		templateLocation := c.String("template-location")
		deploymentTemplatePath := c.String("deployment-template")
		skipDeployment := c.Bool("skip-deployment")
		bundlePropertiesPath := c.Path("bundle-properties")

		layers, buildParameters, err := validateBundle(bundlePath)
		if err != nil {
			return errors.Wrap(err, "bundle validation error")
		}

		// check if azure CLI is installed locally
		if _, err := exec.LookPath("az"); err != nil {
			return fmt.Errorf("az command not found. Install the Azure CLI and restart this terminal: https://learn.microsoft.com/en-us/cli/azure/install-azure-cli (%w)", err)
		}

		// check if logged into the azure CLI
		fmt.Printf("Checking if you are logged in to the Azure CLI...")
		azureAccountList, err := lib.ExecuteAsParseAsJSON[[]lib.AzAccount](c.Context, "az", "account", "list", "--only-show-errors")
		if err != nil {
			return errors.Wrap(err, "Azure CLI login check failed. Make sure you are logged in. You can run 'az login' (https://learn.microsoft.com/en-us/cli/azure/authenticate-azure-cli?view=azure-cli-latest#sign-into-azure-with-azure-cli)")
		}
		color.Green("[DONE]\n")

		// check if subscription is in list of accounts
		var foundSubscription *lib.AzAccount
		for _, azureAccount := range azureAccountList {
			if azureAccount.SubscriptionId == subscriptionId {
				foundSubscription = to.Ptr(azureAccount)
				break
			}
		}
		if foundSubscription == nil {
			return errors.New("Azure CLI is not logged in or logged into the wrong tenant. Try running 'az login'.")
		} else {
			fmt.Printf("Working in Subscription: %s (%s)\n", color.GreenString(foundSubscription.Name), subscriptionId)
		}

		fmt.Println()

		baseImage, err := selectBaseImage(layers, layerBaseImageName)
		if err != nil {
			return errors.Wrap(err, "Failed to select a base image to build from")
		}

		fmt.Println()

		// list image definitions in the gallery
		imageDefinitions, err := lib.ExecuteAsParseAsJSON[[]lib.AzImageDefinition](c.Context, "az", "sig", "image-definition", "list", "-r", imageGallery, "-g", resourceGroup, "--subscription", subscriptionId)
		if err != nil {
			return errors.Wrapf(err, "failed to list Image Definitions in the Image Gallery: %s/%s", resourceGroup, imageGallery)
		}

		if imageDefinition == "" {
			var err error
			imageDefinition, err = selectImageDefinition(imageDefinitions, foundSubscription.TenantId, subscriptionId, resourceGroup, imageGallery)
			if err != nil {
				return errors.Wrap(err, "failed to select Image Definition")
			}
		} else {
			found := false
			for _, def := range imageDefinitions {
				if def.Name == imageDefinition {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("the specified Image Definition (%s) doesn't exist in the Image Gallery (%s)", imageDefinition, imageGallery)
			}
		}
		fmt.Printf("Target Image Definition: ")
		color.Green("%s/%s", imageGallery, imageDefinition)

		fmt.Println()

		if err := writeBundleProperties(layers, buildParameters.Layers, bundlePropertiesPath); err != nil {
			return errors.Wrap(err, "failed to write bundle properties file")
		}

		fmt.Printf("Uploading bundle properties to Azure Storage Account %s/%s...\n", storageAccount, blobContainer)
		bundlePropsBlobName := fmt.Sprintf("bundle-%s-%s.json", imageDefinition, nowFormatted)
		if err := uploadBundle(c.Context, storageAccount, blobContainer, bundlePropertiesPath, bundlePropsBlobName); err != nil {
			return errors.Wrap(err, "failed to upload bundle properties to Azure Storage Account")
		}
		bundlePropsBlobUri := fmt.Sprintf("https://%s.blob.core.windows.net/%s/%s", storageAccount, blobContainer, bundlePropsBlobName)

		fmt.Println()

		fmt.Printf("Calculating bundle hash and size...")
		hash, err := calcBundleShaAndSize(bundlePath)
		if err != nil {
			return errors.Wrapf(err, "failed to calculate hash and size of bundle %s", bundlePath)
		}
		hashHex := hex.EncodeToString(hash)
		color.Green("[DONE] (sha256: %s)", hashHex)

		fmt.Printf("Uploading bundle archive to Azure Storage Account %s/%s...\n", storageAccount, blobContainer)
		bundleArchiveBlobName := fmt.Sprintf("bundle-%s.zip", hashHex)
		if err := uploadBundle(c.Context, storageAccount, blobContainer, bundlePath, bundleArchiveBlobName); err != nil {
			return errors.Wrap(err, "failed to upload bundle archive to Azure Storage Account")
		}

		fmt.Println("")

		if templateName == "" {
			templateName = fmt.Sprintf("%s-%s", imageDefinition, time.Now().Format("2006-01-02_15-04-05"))
			fmt.Printf("Defaulting template name to %s. Overwrite using --template-name.\n", templateName)
		} else {
			fmt.Printf("Using template name: %s (Note: overwriting an existing template will fail)\n", templateName)
		}

		imageTemplate := buildImageTemplate(
			templateName,
			templateLocation,
			managedIdentity,
			int32(buildTimeout),
			start,
			hashHex,
			fmt.Sprintf("https://%s.blob.core.windows.net/%s/%s", storageAccount, blobContainer, bundleArchiveBlobName),
			builderVmSize,
			int32(builderDiskSize),
			baseImage,
			fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Compute/galleries/%s/images/%s", subscriptionId, resourceGroup, imageGallery, imageDefinition),
			int32(replicationCount),
			optimizeImage,
			excludeFromLatest,
			replicationRegions,
			bundlePropsBlobUri,
		)

		deploymentTemplate := buildDeploymentTemplate(imageTemplate)
		deploymentTemplateBytes, err := json.MarshalIndent(deploymentTemplate, "", "    ")
		if err != nil {
			return errors.Wrap(err, "failed to stringify deployment template")
		}

		fmt.Println("")

		fmt.Printf("Writing the Image Template deployment to %s...", deploymentTemplatePath)

		deploymentTemplateFile, err := os.Create(deploymentTemplatePath)
		if err != nil {
			return errors.Wrap(err, "failed to create deployment template file")
		}
		defer deploymentTemplateFile.Close()

		if _, err := deploymentTemplateFile.Write(deploymentTemplateBytes); err != nil {
			return errors.Wrap(err, "failed to write deployment template to disk")
		}
		color.Green("[DONE]")

		fmt.Println()

		if skipDeployment {
			fmt.Println("You opted to skip the deployment of the Image Template")
			fmt.Println("Once you have inspected and/or modified the template, you can deploy it using the Azure CLI")
			fmt.Printf("az deployment group create --resource-group %s --template-file %s --subscription %s\n", resourceGroup, deploymentTemplatePath, subscriptionId)
		} else {
			fmt.Println("Deploying Image Template to Azure...")

			cmd := exec.CommandContext(c.Context, "az", "deployment", "group", "create",
				"--subscription", subscriptionId,
				"--resource-group", resourceGroup,
				"--template-file", deploymentTemplatePath,
				"--no-prompt", "true",
				"--only-show-errors",
				"-o", "none")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr

			if err := cmd.Run(); err != nil {
				return errors.Wrap(err, "failed to create deployment template")
			}

			color.HiGreen("Image Template deployed successfully. You can now see the image builder in the Azure Portal: https://portal.azure.com/#view/Microsoft_Azure_WVD/WvdManagerMenuBlade/~/customImageTemplate")
		}

		return nil
	},
}

func validateBundle(bundlePath string) ([]avdimagetypes.V2LayerProperties, *avdimagetypes.V2BuildParameters, error) {
	archive, err := zip.OpenReader(bundlePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("bundle file does not exist: %s", bundlePath)
		}
		return nil, nil, errors.Wrap(err, "failed to check if bundle file exists")
	}
	defer archive.Close()

	layerNames := make(map[string]struct{})
	filenames := make(map[string]*zip.File)
	for _, f := range archive.File {
		filenames[f.Name] = f

		// treat every top-level folder as a layer-name
		layer, _, found := strings.Cut(f.Name, string(filepath.Separator))
		if found {
			layerNames[layer] = struct{}{}
		}
	}

	var validationErrors []error

	if len(layerNames) == 0 {
		validationErrors = append(validationErrors, fmt.Errorf("bundle file does not contain any layers"))
	}

	if _, ok := filenames[embeddedfiles.V2ExecuteScriptFilename]; !ok {
		validationErrors = append(validationErrors, fmt.Errorf("bundle does not contain the expected 'execute' script (%s)", embeddedfiles.V2ExecuteScriptFilename))
	}

	var buildParameters *avdimagetypes.V2BuildParameters
	if zipFile, ok := filenames[schema.V2BuildParametersFilename]; !ok {
		validationErrors = append(validationErrors, fmt.Errorf("bundle does not contain the expected build parameters files (%s)", schema.V2BuildParametersFilename))
	} else {
		file, err := zipFile.Open()
		if err != nil {
			return nil, nil, errors.Wrap(err, "failed to open build parameters file")
		}
		defer file.Close()

		data, err := io.ReadAll(file)
		if err != nil {
			return nil, nil, errors.Wrap(err, "failed to read build parameters file")
		}

		if err := lib.ValidateAVDImageType(avdimagetypes.V2BuildParametersDefinition, data); err != nil {
			return nil, nil, errors.Wrap(err, "invalid build parameters file")
		}

		if err := json.Unmarshal(data, &buildParameters); err != nil {
			return nil, nil, errors.Wrap(err, "failed to parse build parameters file")
		}
	}

	layers := make([]avdimagetypes.V2LayerProperties, 0, len(layerNames))
	for layerName := range layerNames {
		layerFs, err := fs.Sub(archive, layerName)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "failed to open layer %s", layerName)
		}
		propsBytes, _, err := lib.ReadJSONOrJSON5AsJSON(layerFs, layerPropertiesFilename)
		if err != nil {
			validationErrors = append(validationErrors, errors.Wrapf(err, "failed to read properties file of layer %s", layerName))
			continue
		} else {
			if err := lib.ValidateAVDImageType(avdimagetypes.V2LayerPropertiesDefinition, propsBytes); err != nil {
				validationErrors = append(validationErrors, errors.Wrapf(err, "invalid properties file of layer %s", layerName))
			} else {
				var properties avdimagetypes.V2LayerProperties
				if err := json.Unmarshal(propsBytes, &properties); err != nil {
					return nil, nil, errors.Wrapf(err, "failed to parse layer %s", layerName)
				}

				layers = append(layers, properties)
			}
		}
	}

	if len(validationErrors) > 0 {
		return nil, nil, stdErr.Join(validationErrors...)
	}

	return layers, buildParameters, nil
}

func selectBaseImage(layers []avdimagetypes.V2LayerProperties, preselectedLayerName string) (*avdimagetypes.V2LayerPropertiesBaseImage, error) {
	baseImages := make(map[string]*avdimagetypes.V2LayerPropertiesBaseImage, len(layers))
	var layerIdxsWithBaseImage []int
	for i, layer := range layers {
		baseImages[layer.Name] = layer.BaseImage
		if layer.BaseImage != nil {
			layerIdxsWithBaseImage = append(layerIdxsWithBaseImage, i)
		}
	}

	if preselectedLayerName != "" {
		baseImage, found := baseImages[preselectedLayerName]
		if !found {
			return nil, fmt.Errorf("preselected layer %s does not exist", preselectedLayerName)
		}

		if baseImage == nil {
			return nil, fmt.Errorf("selected layer %s does not have a base image configured", preselectedLayerName)
		}

		fmt.Printf("Using the base image from layer %s: \n    %s\n", preselectedLayerName, baseImageToString(defaultBaseImage))

		return baseImage, nil
	}

	switch len(layerIdxsWithBaseImage) {
	case 0:
		fmt.Printf("No layer explicitly defines a base image, so the default base image will be used: \n    %s\n", baseImageToString(defaultBaseImage))
		return defaultBaseImage, nil
	case 1:
		layer := layers[layerIdxsWithBaseImage[0]]
		fmt.Printf("The layer %s defines the base image that will be used: \n    %s\n", layer.Name, baseImageToString(layer.BaseImage))
		return layer.BaseImage, nil
	default:
		fmt.Println("Multiple layers define a base image")

		options := make([]string, len(layerIdxsWithBaseImage))
		for i, layerIdx := range layerIdxsWithBaseImage {
			layer := layers[layerIdx]
			options[i] = fmt.Sprintf("    - %d: Layer %s\n: %s", i+1, layer.Name, baseImageToString(layer.BaseImage))
		}
		idx, err := lib.PromptEnum("Select the base image to use", options, "", nil)
		if err != nil {
			return nil, err
		}

		layer := layers[layerIdxsWithBaseImage[idx]]
		fmt.Printf("Layer %s selected: \n%s\n", layer.Name, baseImageToString(layer.BaseImage))
		return layer.BaseImage, nil
	}
}

func selectImageDefinition(existingImageDefinitions []lib.AzImageDefinition, tenantId, subscriptionId, rgName, galleryName string) (name string, err error) {
	createURL := fmt.Sprintf("https://portal.azure.com/#@%s/resource/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Compute/galleries/%s/overview", tenantId, subscriptionId, rgName, galleryName)

	if len(existingImageDefinitions) == 0 {
		color.Yellow("No existing image definitions found")
		color.Yellow("Create a new one here: %s", createURL)
		return "", errors.New("no existing image definitions found")
	}

	color.Cyan("You did not specify an Image Definition, please select an existing one or create a new one in the Azure Portal:")
	color.Cyan("To create a new one: %s", createURL)
	color.Cyan("To select an image definition non-interactively, pass the --image-definition flag")
	fmt.Println()

	options := make([]string, len(existingImageDefinitions))
	for i, def := range existingImageDefinitions {
		options[i] = def.Name
	}
	var defaultIdx *int
	if len(options) == 1 {
		idx := 0
		defaultIdx = &idx
	}
	idx, err := lib.PromptEnum(color.YellowString("Select an Image Definition"), options, "", defaultIdx)
	if err != nil {
		return "", err
	}

	return existingImageDefinitions[idx].Name, nil
}

func calcBundleShaAndSize(filepath string) ([]byte, error) {
	f, err := os.Open(filepath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open file")
	}
	defer f.Close()

	sha := sha256.New()
	if _, err := io.Copy(sha, f); err != nil {
		return nil, errors.Wrap(err, "failed to calculate sha256")
	}
	return sha.Sum(nil), nil
}

func uploadBundle(ctx context.Context, storageAccount, blobContainer, bundlePath, bundleFileName string) error {
	type existsOut struct {
		Exists bool `json:"exists"`
	}
	exists, err := lib.ExecuteAsParseAsJSON[*existsOut](ctx, "az", "storage", "blob", "exists",
		"--account-name", storageAccount,
		"--container-name", blobContainer,
		"-n", bundleFileName,
		"--auth-mode", "login",
		"--only-show-errors")
	if err != nil {
		return errors.Wrap(err, "failed to check if the bundle is already uploaded")
	}

	if exists.Exists {
		fmt.Println("Bundle is already uploaded")
		return nil
	}

	uploadCmd := exec.CommandContext(ctx, "az", "storage", "blob", "upload",
		"-f", bundlePath,
		"-c", blobContainer,
		"-n", bundleFileName,
		"--account-name", storageAccount,
		"--only-show-errors",
		"--auth-mode", "login",
		"-o", "none")
	uploadCmd.Stderr = os.Stderr
	uploadCmd.Stdout = os.Stdout
	if err := uploadCmd.Run(); err != nil {
		return errors.Wrap(err, "failed to upload bundle")
	}

	fmt.Println("Done uploading bundle")
	return nil
}

func buildImageTemplate(
	name string,
	location string,
	managedIdentityId string,
	buildTimeoutMinutes int32,
	autoStart bool,

	bundleHashHex string,
	bundleUri string,

	builderVmSize string,
	builderDiskSize int32,

	baseImage *avdimagetypes.V2LayerPropertiesBaseImage,

	targetGalleryImageId string,
	replicateCount int32,
	optimizeImage bool,
	excludeFromLatest bool,
	targetRegions []string,

	bundlePropertiesUri string,
) *armvirtualmachineimagebuilder.ImageTemplate {
	return &armvirtualmachineimagebuilder.ImageTemplate{
		Identity: &armvirtualmachineimagebuilder.ImageTemplateIdentity{
			Type: to.Ptr(armvirtualmachineimagebuilder.ResourceIdentityTypeUserAssigned),
			UserAssignedIdentities: map[string]*armvirtualmachineimagebuilder.UserAssignedIdentity{
				managedIdentityId: {},
			},
		},
		Location: to.Ptr(location),
		Properties: &armvirtualmachineimagebuilder.ImageTemplateProperties{
			Distribute: []armvirtualmachineimagebuilder.ImageTemplateDistributorClassification{
				buildTemplateDistributor(targetGalleryImageId, targetRegions, replicateCount, excludeFromLatest, bundlePropertiesUri),
			},
			Source: baseImageSourceToTemplateSource(baseImage),
			AutoRun: (func() *armvirtualmachineimagebuilder.ImageTemplateAutoRun {
				state := armvirtualmachineimagebuilder.AutoRunStateAutoRunDisabled
				if autoStart {
					state = armvirtualmachineimagebuilder.AutoRunStateAutoRunEnabled
				}
				return &armvirtualmachineimagebuilder.ImageTemplateAutoRun{
					State: &state,
				}
			})(),
			BuildTimeoutInMinutes: to.Ptr(buildTimeoutMinutes),
			Customize:             buildCustomizationSteps(bundleHashHex, bundleUri),
			VMProfile: &armvirtualmachineimagebuilder.ImageTemplateVMProfile{
				OSDiskSizeGB: to.Ptr(builderDiskSize),
				VMSize:       to.Ptr(builderVmSize),
			},
			Optimize: &armvirtualmachineimagebuilder.ImageTemplatePropertiesOptimize{
				VMBoot: &armvirtualmachineimagebuilder.ImageTemplatePropertiesOptimizeVMBoot{
					State: (func() *armvirtualmachineimagebuilder.VMBootOptimizationState {
						if optimizeImage {
							return to.Ptr(armvirtualmachineimagebuilder.VMBootOptimizationStateEnabled)
						} else {
							return to.Ptr(armvirtualmachineimagebuilder.VMBootOptimizationStateDisabled)
						}
					})(),
				},
			},
		},
		Tags: map[string]*string{
			// so it shows up in the AVD Image Builder page
			"AVD_IMAGE_TEMPLATE": to.Ptr("AVD_IMAGE_TEMPLATE"),
		},
		Name: to.Ptr(name),
		Type: to.Ptr("Microsoft.VirtualMachineImages/imageTemplates"),
	}
}

func baseImageSourceToTemplateSource(baseImage *avdimagetypes.V2LayerPropertiesBaseImage) armvirtualmachineimagebuilder.ImageTemplateSourceClassification {
	switch {
	case baseImage.ManagedImage != nil:
		return &armvirtualmachineimagebuilder.ImageTemplateManagedImageSource{
			ImageID: to.Ptr(baseImage.ManagedImage.ImageID),
			Type:    to.Ptr(string(baseImage.ManagedImage.Type)),
		}
	case baseImage.SharedImageVersion != nil:
		return &armvirtualmachineimagebuilder.ImageTemplateSharedImageVersionSource{
			ImageVersionID: to.Ptr(baseImage.SharedImageVersion.ImageVersionID),
			Type:           to.Ptr(string(baseImage.SharedImageVersion.Type)),
			ExactVersion:   nil,
		}
	case baseImage.PlatformImage != nil:
		return &armvirtualmachineimagebuilder.ImageTemplatePlatformImageSource{
			Type:  to.Ptr(string(baseImage.PlatformImage.Type)),
			Offer: to.Ptr(baseImage.PlatformImage.Offer),
			PlanInfo: (func() *armvirtualmachineimagebuilder.PlatformImagePurchasePlan {
				if baseImage.PlatformImage.PlanInfo == nil {
					return nil
				}
				return &armvirtualmachineimagebuilder.PlatformImagePurchasePlan{
					PlanName:      to.Ptr(baseImage.PlatformImage.PlanInfo.PlanName),
					PlanProduct:   to.Ptr(baseImage.PlatformImage.PlanInfo.PlanProduct),
					PlanPublisher: to.Ptr(baseImage.PlatformImage.PlanInfo.PlanPublisher),
				}
			})(),
			Publisher: to.Ptr(baseImage.PlatformImage.Publisher),
			SKU:       to.Ptr(baseImage.PlatformImage.Sku),
			Version:   to.Ptr(baseImage.PlatformImage.Version),
		}
	default:
		panic("unknown base image type")
	}
}

func buildTemplateDistributor(galleryImageId string, targetRegions []string, replicateCount int32, excludeFromLatest bool, bundlePropertiesUri string) *armvirtualmachineimagebuilder.ImageTemplateSharedImageDistributor {
	return &armvirtualmachineimagebuilder.ImageTemplateSharedImageDistributor{
		GalleryImageID:    to.Ptr(galleryImageId),
		RunOutputName:     to.Ptr("gallery"),
		Type:              to.Ptr("SharedImage"),
		ExcludeFromLatest: to.Ptr(excludeFromLatest),
		TargetRegions: (func() []*armvirtualmachineimagebuilder.TargetRegion {
			regions := make([]*armvirtualmachineimagebuilder.TargetRegion, len(targetRegions))
			for i, region := range targetRegions {
				regionName := region
				regions[i] = &armvirtualmachineimagebuilder.TargetRegion{
					Name:               to.Ptr(regionName),
					ReplicaCount:       to.Ptr(replicateCount),
					StorageAccountType: nil,
				}
			}
			return regions
		})(),
		Versioning: nil, // could be useful in the future
		ArtifactTags: map[string]*string{
			"SY_BUNDLE_URL": to.Ptr(bundlePropertiesUri),
		},
	}
}

func buildCustomizationSteps(bundleHashHex string, bundleSourceUri string) []armvirtualmachineimagebuilder.ImageTemplateCustomizerClassification {
	const imageBundleZipFilepath = `C:\image_bundle.zip`
	const imageBundleFilepath = `C:\image_bundle`

	return []armvirtualmachineimagebuilder.ImageTemplateCustomizerClassification{
		// download zip
		&armvirtualmachineimagebuilder.ImageTemplateFileCustomizer{
			Type:           to.Ptr("File"),
			Destination:    to.Ptr(imageBundleZipFilepath),
			Name:           to.Ptr("Download bundle"),
			SHA256Checksum: to.Ptr(bundleHashHex),
			SourceURI:      to.Ptr(bundleSourceUri),
		},

		// Run PowerShell
		&armvirtualmachineimagebuilder.ImageTemplatePowerShellCustomizer{
			Type: to.Ptr("PowerShell"),
			Inline: to.SliceOfPtrs(
				`Write-Host "Set error action to 'stop'"`,
				`$ErrorActionPreference = "Stop"`,
				`Write-Host "Extracting bundle archive"`,
				fmt.Sprintf(`Expand-Archive -LiteralPath '%s' -DestinationPath '%s'`, imageBundleZipFilepath, imageBundleFilepath),
				`Write-Host "Entering the bundle directory"`,
				fmt.Sprintf(`Push-Location "%s"`, imageBundleFilepath),
				`Write-Host "Executing bundle"`,
				fmt.Sprintf(`& "./%s" -ScanForDirectories -Force`, embeddedfiles.V2ExecuteScriptFilename),
				`if (!$?) {Write-Error "The bundle execution failed"; exit 5}`,
				`Write-Host Exiting the bundle directory`,
				`Pop-Location`,
				`Write-Host "Removing bundle directory and archive"`,
				fmt.Sprintf(`Remove-Item -Path "%s", "%s" -Recurse`, imageBundleZipFilepath, imageBundleFilepath),
				`Write-Host "Adjusting deprovisioning script"`,
				// based on: https://raw.githubusercontent.com/Azure/RDS-Templates/master/CustomImageTemplateScripts/CustomImageTemplateScripts_2024-03-27/AdminSysPrep.ps1
				`((Get-Content -path C:\\DeprovisioningScript.ps1 -Raw) -replace 'Sysprep.exe /oobe /generalize /quiet /quit','Sysprep.exe /oobe /generalize /quit /mode:vm' ) | Set-Content -Path C:\\DeprovisioningScript.ps1`,
				`Write-Host "Ready for sysprep"`,
			),
			Name:        to.Ptr("build"),
			RunAsSystem: to.Ptr(true),
			RunElevated: to.Ptr(true),
		},

		// not restarting may cause sysprep to fail (stuck in IMAGE_STATE_COMPLETE loop)
		&armvirtualmachineimagebuilder.ImageTemplateRestartCustomizer{
			Type: to.Ptr("WindowsRestart"),
			Name: to.Ptr("Restart before sysprep"),
		},
	}
}

func buildDeploymentTemplate(imageTemplate *armvirtualmachineimagebuilder.ImageTemplate) map[string]any {
	resource := lib.JSONCombinedMarshaller{
		Objects: []any{imageTemplate, struct {
			ApiVersion string `json:"apiVersion"`
		}{ApiVersion: "2024-02-01"}},
	}

	return map[string]any{
		"$schema":        "https://schema.management.azure.com/schemas/2019-04-01/deploymentTemplate.json#",
		"contentVersion": "1.0.0.0",
		"resources": []any{
			resource,
		},
	}
}

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
	"github.com/friendsofgo/errors"
	"github.com/schoolyear/avd-cli/embeddedfiles"
	"github.com/schoolyear/avd-cli/lib"
	avdimagetypes "github.com/schoolyear/avd-image-types"
	"github.com/urfave/cli/v2"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"strconv"
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
			Name:     "template-name",
			Usage:    "Overwrite the name of the Image Template (defaults to [image-definition]-[timestamp]). Note that the operation will fail if this name is already taken.",
			Required: true,
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
		bundle := c.Path("bundle")
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
		templateLocation := c.String("template-location")
		deploymentTemplatePath := c.String("deployment-template")
		skipDeployment := c.Bool("skip-deployment")
		bundlePropertiesPath := c.Path("bundle-properties")

		layers, err := validateBundle(bundle)
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
		fmt.Printf("[DONE]\n")

		// check if subscription is in list of accounts
		foundSubscriptionName := ""
		for _, azureAccount := range azureAccountList {
			if azureAccount.SubscriptionId == subscriptionId {
				foundSubscriptionName = azureAccount.Name
				break
			}
		}
		if foundSubscriptionName == "" {
			return errors.New("Azure CLI is not logged in to a tenant with the specified subscription-id")
		} else {
			fmt.Printf("Working in Subscription: %s (%s)\n", foundSubscriptionName, subscriptionId)
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
			imageDefinition, err = selectImageDefinition(imageDefinitions)
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
				return fmt.Errorf("The specified Image Definition (%s) doesn't exist in the Image Gallery (%s)\n", imageDefinition, imageGallery)
			}
		}
		fmt.Printf("Target Image Definition: %s/%s\n", imageGallery, imageDefinition)

		fmt.Println()

		fmt.Printf("Calculating bundle hash and size...")
		hash, err := calcBundleShaAndSize(bundle)
		if err != nil {
			return errors.Wrapf(err, "failed to calculate hash and size of bundle %s", bundle)
		}
		hashHex := hex.EncodeToString(hash)
		fmt.Printf("[DONE] (sha256: %s)\n", hashHex)

		fmt.Printf("Uploading bundle to Azure Storage Account %s/%s...\n", storageAccount, blobContainer)
		bundleBlobName := fmt.Sprintf("bundle-%s.zip", hashHex)
		if err := uploadBundle(c.Context, storageAccount, blobContainer, bundle, bundleBlobName); err != nil {
			return errors.Wrap(err, "failed to upload bundle to Azure Storage Account")
		}

		fmt.Println("")

		if templateName == "" {
			templateName = fmt.Sprintf("%s-%s", imageDefinition, time.Now().Format(time.DateTime))
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
			fmt.Sprintf("https://%s.blob.core.windows.net/%s/%s", storageAccount, blobContainer, bundleBlobName),
			builderVmSize,
			int32(builderDiskSize),
			baseImage,
			fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Compute/galleries/%s/images/%s", subscriptionId, resourceGroup, imageGallery, imageDefinition),
			int32(replicationCount),
			excludeFromLatest,
			replicationRegions,
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
		fmt.Println("[DONE]")

		fmt.Println()
		if err := writeBundleProperties(layers, bundlePropertiesPath); err != nil {
			return errors.Wrap(err, "failed to write bundle properties file")
		}
		fmt.Println("")

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

			fmt.Println("Image Template deployed successfully. You can now see the image builder in the Azure Portal: https://portal.azure.com/#view/Microsoft_Azure_WVD/WvdManagerMenuBlade/~/customImageTemplate")
		}

		return nil
	},
}

func validateBundle(bundlePath string) ([]avdimagetypes.V2LayerProperties, error) {
	archive, err := zip.OpenReader(bundlePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("bundle file does not exist: %s", bundlePath)
		}
		return nil, errors.Wrap(err, "failed to check if bundle file exists")
	}
	defer archive.Close()

	layerNames := make(map[string]struct{})
	filenames := make(map[string]struct{})
	for _, f := range archive.File {
		filenames[f.Name] = struct{}{}

		// treat every top-level folder as a layer-name
		layer, _, found := strings.Cut(f.Name, "/")
		if found {
			layerNames[layer] = struct{}{}
		}
	}

	var validationErrors []error

	if len(layerNames) == 0 {
		validationErrors = append(validationErrors, fmt.Errorf("bundle file does not contain any layers"))
	}

	if _, ok := filenames[embeddedfiles.V2ExecuteScriptFilename]; !ok {
		validationErrors = append(validationErrors, fmt.Errorf("bundle does not contain the expected execute script (%s)", embeddedfiles.V2ExecuteScriptFilename))
	}

	layers := make([]avdimagetypes.V2LayerProperties, 0, len(layerNames))
	for layerName := range layerNames {
		layerFs, err := fs.Sub(archive, layerName)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to open layer %s", layerName)
		}
		propsBytes, _, err := lib.ReadJSONOrJSON5AsJSON(layerFs, layerPropertiesFilename)
		if err != nil {
			validationErrors = append(validationErrors, errors.Wrapf(err, "failed to read properties file of layer %s", layerName))
			continue
		} else {
			validationResult, err := avdimagetypes.ValidateDefinition(avdimagetypes.V2LayerPropertiesDefinition, propsBytes)
			if err != nil {
				validationErrors = append(validationErrors, errors.Wrapf(err, "failed to validate layer %s", layerName))
			} else if !validationResult.Valid() {
				resultErrors := validationResult.Errors()
				propsValidationErrors := make([]error, len(resultErrors))
				for i, validationError := range resultErrors {
					propsValidationErrors[i] = errors.New(validationError.String())
				}

				propsValidationErr := errors.Wrapf(stdErr.Join(propsValidationErrors...), "invalid properties file of layer %s", layerName)
				validationErrors = append(validationErrors, propsValidationErr)
			} else {
				var properties avdimagetypes.V2LayerProperties
				if err := json.Unmarshal(propsBytes, &properties); err != nil {
					return nil, errors.Wrapf(err, "failed to parse layer %s", layerName)
				}

				layers = append(layers, properties)
			}
		}
	}

	if len(validationErrors) > 0 {
		return nil, stdErr.Join(validationErrors...)
	}

	return layers, nil
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

		fmt.Printf("Using the base image from layer %s: \n\t%s\n", preselectedLayerName, baseImageToString(defaultBaseImage))

		return baseImage, nil
	}

	switch len(layerIdxsWithBaseImage) {
	case 0:
		fmt.Printf("No layer explicitly defines a base image, so the default base image will be used: \n\t%s\n", baseImageToString(defaultBaseImage))
		return defaultBaseImage, nil
	case 1:
		layer := layers[layerIdxsWithBaseImage[0]]
		fmt.Printf("The layer %s defines the base image that will be used: \n\t%s\n", layer.Name, baseImageToString(layer.BaseImage))
		return layer.BaseImage, nil
	default:
		fmt.Println("Multiple layers define a base image")
		for i, layerIdx := range layerIdxsWithBaseImage {
			layer := layers[layerIdx]
			fmt.Printf("\t- %d: Layer %s\n: %s", i+1, layer.Name, baseImageToString(layer.BaseImage))
		}
		selectionStr, err := lib.PromptUserInput("[Select the base image to use]: ")
		if err != nil {
			return nil, errors.Wrap(err, "failed to read the user input")
		}
		selection, err := strconv.Atoi(selectionStr)
		if err != nil {
			return nil, errors.Wrap(err, "invalid input")
		}

		if selection > len(layerIdxsWithBaseImage) || selection < 1 {
			return nil, fmt.Errorf("invalid selection %d", selection)
		}

		layer := layers[layerIdxsWithBaseImage[selection-1]]
		fmt.Printf("Layer %s selected: \n%s\n", layer.Name, baseImageToString(layer.BaseImage))
		return layer.BaseImage, nil
	}
}

func selectImageDefinition(existingImageDefinitions []lib.AzImageDefinition) (name string, err error) {
	fmt.Println("You did not specify an Image Definition, please select an existing one")
	fmt.Println("You can create a Image Definition in the Azure Portal")
	fmt.Println("To select an image definition non-interactively, pass the --image-definition flag")
	for i, def := range existingImageDefinitions {
		fmt.Printf("[%d]: %s\n", i+1, def.Name)
	}
	selectionStr, err := lib.PromptUserInput("[Select an Image Definition]: ")
	if err != nil {
		return "", errors.Wrap(err, "failed to read the user input")
	}
	selection, err := strconv.Atoi(selectionStr)
	if err != nil {
		return "", errors.Wrap(err, "invalid input")
	}

	if selection > len(existingImageDefinitions) || selection < 1 {
		return "", fmt.Errorf("invalid selection %d", selection)
	}

	return existingImageDefinitions[selection-1].Name, nil
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
	excludeFromLatest bool,
	targetRegions []string,
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
				buildTemplateDistributor(targetGalleryImageId, targetRegions, replicateCount, excludeFromLatest),
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

func buildTemplateDistributor(galleryImageId string, targetRegions []string, replicateCount int32, excludeFromLatest bool) *armvirtualmachineimagebuilder.ImageTemplateSharedImageDistributor {
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

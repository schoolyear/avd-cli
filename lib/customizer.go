package lib

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/virtualmachineimagebuilder/armvirtualmachineimagebuilder/v2"
	avdimagetypes "github.com/schoolyear/avd-image-types"
)

func CustomizerToArmImageTemplateCustomizer(customizer avdimagetypes.V2Customizer) armvirtualmachineimagebuilder.ImageTemplateCustomizerClassification {
	switch {
	case customizer.WindowsUpdate != nil:
		updateCustomizer := customizer.WindowsUpdate
		return &armvirtualmachineimagebuilder.ImageTemplateWindowsUpdateCustomizer{
			Type: (*string)(&updateCustomizer.Type),
			Name: &updateCustomizer.Name,
		}
	case customizer.WindowsRestart != nil:
		restartCustomizer := customizer.WindowsRestart
		return &armvirtualmachineimagebuilder.ImageTemplateRestartCustomizer{
			Type: (*string)(&restartCustomizer.Type),
			Name: &restartCustomizer.Name,
		}
	case customizer.File != nil:
		fileCustomizer := customizer.File
		armFileCustomizer := &armvirtualmachineimagebuilder.ImageTemplateFileCustomizer{
			Type:        (*string)(&fileCustomizer.Type),
			Name:        &fileCustomizer.Name,
			Destination: &fileCustomizer.Destination,
			SourceURI:   &fileCustomizer.SourceURI,
		}

		if fileCustomizer.Sha256Checksum != "" {
			armFileCustomizer.SHA256Checksum = &fileCustomizer.Sha256Checksum
		}

		return armFileCustomizer
	case customizer.PowerShell != nil:
		powershellCustomizer := customizer.PowerShell
		armPowershellCustomizer := &armvirtualmachineimagebuilder.ImageTemplatePowerShellCustomizer{
			Type:        (*string)(&powershellCustomizer.Type),
			Name:        &powershellCustomizer.Name,
			RunAsSystem: &powershellCustomizer.System,
			RunElevated: &powershellCustomizer.Elevated,
		}

		if len(powershellCustomizer.Inline) > 0 {
			armPowershellCustomizer.Inline = to.SliceOfPtrs(powershellCustomizer.Inline...)
		}

		if len(powershellCustomizer.ValidExitCodes) > 0 {
			validExitCodes := make([]*int32, len(powershellCustomizer.ValidExitCodes))
			for i, code := range powershellCustomizer.ValidExitCodes {
				validExitCodes[i] = to.Ptr(int32(code))
			}

			armPowershellCustomizer.ValidExitCodes = validExitCodes
		}

		if powershellCustomizer.Script != "" {
			armPowershellCustomizer.ScriptURI = &powershellCustomizer.Script
		}

		if powershellCustomizer.Sha256Checksum != "" {
			armPowershellCustomizer.SHA256Checksum = &powershellCustomizer.Sha256Checksum
		}

		return armPowershellCustomizer
	default:
		panic("invalid customizer")
	}
}

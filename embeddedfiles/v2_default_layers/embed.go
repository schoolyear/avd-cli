package v2_default_layers

import (
	"embed"
	"fmt"
	"maps"
	"strings"

	"github.com/schoolyear/avd-cli/lib"
	avdimagetypes "github.com/schoolyear/avd-image-types"
	"zgo.at/zstd/ztype"
)

//go:embed win10_22h2/*
var Win1022h2 embed.FS

//go:embed win11_24h2_preview/*
var Win1124h2Preview embed.FS

const (
	Win1022h2Name     = "win10-22h2"
	Win1124h2BetaName = "win11-24h2-preview"
)

const DefaultBaseLayerName = Win1022h2Name

var BaseLayers = map[string]BaseLayer{
	Win1022h2Name: {
		Path: "win10_22h2",
		FS:   Win1022h2,
		Warning: ztype.NewOptional(BaseLayerWarning{
			Header: "Deprecation Notice",
			Message: `Windows 10 will be deprecated by Azure in April 2026.
This mean you cannot build new images using Windows 10 and support is not available.
On March 15 2026, avdcli will default to Windows 11.`,
		}),
		BaseImageChecker: PlatformSkuStringContainsChecker{SkuSubstring: "win10"},
	},
	Win1124h2BetaName: {
		Path:             "win11_24h2_preview",
		FS:               Win1124h2Preview,
		BaseImageChecker: PlatformSkuStringContainsChecker{SkuSubstring: "win11"},
	},
}

var BaseLayerShortnames = lib.CollectSeq(maps.Keys(BaseLayers))

type BaseLayer struct {
	Path             string
	FS               embed.FS
	Warning          ztype.Optional[BaseLayerWarning]
	BaseImageChecker BaseImageChecker
}

type BaseLayerWarning struct {
	Header  string
	Message string
}

type BaseImageChecker interface {
	// ValidateBaseImage checks whether a base layer supports a given base image
	// returns a human-readable error if not
	ValidateBaseImage(baseImage *avdimagetypes.V2BaseImage) error
}

type PlatformSkuStringContainsChecker struct {
	SkuSubstring string
}

func (s PlatformSkuStringContainsChecker) ValidateBaseImage(baseImage *avdimagetypes.V2BaseImage) error {
	if baseImage.PlatformImage == nil {
		return nil
	}

	sku := strings.ToLower(baseImage.PlatformImage.Sku)
	substring := strings.ToLower(s.SkuSubstring)
	if !strings.Contains(sku, substring) {
		return fmt.Errorf("this base layer only supports platform images that contain '%s' in their SKU or non-platform images", s.SkuSubstring)
	}

	return nil
}

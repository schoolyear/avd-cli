package v2_default_layers

import (
	"embed"
	"maps"

	"github.com/schoolyear/avd-cli/lib"
	"zgo.at/zstd/ztype"
)

//go:embed win10_22h2/*
var Win1022h2 embed.FS

//go:embed win11_24h2_beta/*
var Win1124h2Beta embed.FS

const (
	Win1022h2Name     = "win10-22h2"
	Win1124h2BetaName = "win11-24h2-beta"
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
	},
	Win1124h2BetaName: {
		Path: "win11_24h2_beta",
		FS:   Win1124h2Beta,
	},
}

type BaseLayer struct {
	Path    string
	FS      embed.FS
	Warning ztype.Optional[BaseLayerWarning]
}

type BaseLayerWarning struct {
	Header  string
	Message string
}

var BaseLayerShortnames = lib.CollectSeq(maps.Keys(BaseLayers))

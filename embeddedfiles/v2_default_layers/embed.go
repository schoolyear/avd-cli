package v2_default_layers

import (
	"embed"
	"maps"

	"github.com/schoolyear/avd-cli/lib"
	"zgo.at/zstd/ztype"
)

//go:embed win10_22h2/*
var Win1022h2 embed.FS

////go:embed win11/*
//var Win11 embed.FS

const (
	Win1022h2Name = "win10_22h2"
)

const DefaultBaseLayerName = Win1022h2Name

var BaseLayers = map[string]BaseLayer{
	Win1022h2Name: {
		Path: Win1022h2Name,
		FS:   Win1022h2,
		Warning: ztype.NewOptional(BaseLayerWarning{
			Header: "Deprecation Notice",
			Message: `Windows 10 will be deprecated by Azure in April 2026.
This mean you cannot build new images using Windows 10 and support is not available.
On March 15 2026, avdcli will default to Windows 11.`,
		}),
	},
	//"win11": {
	//	Path: "win11",
	//	FS:   Win11,
	//},
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

package static

// these variables are baked in during compilation
var (
	Version     = "v0.0.0"
	ReleaseFile = "unknown-release-file"
)

var (
	GithubRepository = "https://github.com/schoolyear/avd-cli"
)

var (
	GithubImageCommunityOwner     = "schoolyear"
	GithubImageCommunityRepo      = "avd-image-community"
	GithubImageCommunityLayerPath = "layers"
)

var (
	GithubAppClientId = "Iv23liPR35tAaWqoqYYs" // this is not a secret but a public value
)

var (
	KeyringServiceName = "avdcli"
)

type ContextKey int

const (
	CtxUpdatedKey ContextKey = iota
)

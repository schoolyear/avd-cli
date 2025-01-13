package static

// these variables are baked in during compilation
var (
	Version     = "v0.0.0"
	ReleaseFile = "unknown-release-file"
)

var (
	GithubRepository = "https://github.com/schoolyear/avd-cli"
)

type ContextKey int

const (
	CtxUpdatedKey ContextKey = iota
)

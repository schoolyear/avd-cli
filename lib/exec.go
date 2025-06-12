package lib

import (
	"context"
	"encoding/json"
	"github.com/friendsofgo/errors"
	"io"
	"os/exec"
	"regexp"
	"strings"
)

var jsonParseFixRegex = regexp.MustCompile(`(?m)^.*(?:pkg_resources is deprecated as an API|__import__\('pkg_resources'\)).*\n?`)

func ExecuteAsParseAsJSON[T any](ctx context.Context, cmd string, args ...string) (t T, err error) {
	return ExecuteAsParseAsJSONWithStdin[T](ctx, nil, cmd, args...)
}

func ExecuteAsParseAsJSONWithStdin[T any](ctx context.Context, stdin io.Reader, cmd string, args ...string) (t T, err error) {
	command := exec.CommandContext(ctx, cmd, args...)
	command.Stdin = stdin

	out, err := command.CombinedOutput()
	if err != nil {
		return t, errors.Wrapf(err, "failed to execute %s %s: %s", cmd, strings.Join(args, " "), string(out))
	}

	// fix for: https://github.com/azure/azure-cli/issues/31591
	// Azure CLI may output this, which messes with the JSON parsing
	// We check if the output contains it and remove it before parsing it

	// check if the "out" byte array contains a line with "pkg_resources is deprecated as an API" or "__import__('pkg_resources')".
	// use regex to remove those lines
	out = jsonParseFixRegex.ReplaceAll(out, []byte{})

	if err := json.Unmarshal(out, &t); err != nil {
		return t, errors.Wrapf(err, "failed to parse output of %s %s: %s", cmd, strings.Join(args, " "), string(out))
	}

	return t, nil
}

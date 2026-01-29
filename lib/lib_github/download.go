package lib_github

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/schoolyear/avd-cli/static"

	"github.com/friendsofgo/errors"
	"github.com/go-resty/resty/v2"
)

const (
	downloadTimeout    = 10 * time.Second
	downloadRetryCount = 2
	retryWaitTime      = 1 * time.Second
)

func newRestyClient() *resty.Client {
	return resty.New().
		SetTimeout(downloadTimeout).
		SetRetryWaitTime(retryWaitTime).
		SetRetryCount(downloadRetryCount)
}

func DownloadFile(ctx context.Context, url string) ([]byte, error) {
	res, err := newRestyClient().
		R().
		SetContext(ctx).
		Get(url)
	if err != nil {
		return nil, errors.Wrap(err, "failed to make request to download file")
	}

	if !res.IsSuccess() {
		return nil, errors.Errorf("failed to download file (%d): %s", res.StatusCode(), string(res.Body()))
	}

	return res.Body(), nil
}

func FetchLatestVersion(ctx context.Context) (version string, downloadUrl string, err error) {
	releaseTag, err := GetLatestReleaseFromGithub(ctx, static.GithubRepository)
	if err != nil {
		return "", "", errors.Wrapf(err, "failed to fetch latest version for %s", static.GithubRepository)
	}

	downloadUrl = fmt.Sprintf("%s/releases/download/%s/%s", static.GithubRepository, releaseTag, static.ReleaseFile)

	return releaseTag, downloadUrl, nil
}

func GetLatestReleaseFromGithub(ctx context.Context, repositoryUrl string) (string, error) {
	const latestReleasePath = "/releases/latest"
	const releaseTagPathPrefix = "/releases/tag/"

	res, err := newRestyClient().
		SetRedirectPolicy(resty.RedirectPolicyFunc(func(req *http.Request, requests []*http.Request) error {
			if len(requests) == 1 && strings.HasPrefix(req.URL.String(), repositoryUrl+releaseTagPathPrefix) {
				return http.ErrUseLastResponse
			}
			fmt.Println(req.URL.String(), repositoryUrl+releaseTagPathPrefix)
			return fmt.Errorf("refusing to redirect to %s", req.URL.String())
		})).
		R().
		SetContext(ctx).
		Head(repositoryUrl + latestReleasePath)
	if err != nil {
		return "", errors.Wrap(err, "failed to request latest release from Github")
	}

	if !res.IsSuccess() && res.StatusCode() != http.StatusFound {
		return "", fmt.Errorf("failed to request latest release from Github: %v", res.Status())
	}

	locationHeader := res.Header().Get("Location")
	tagPrefix := repositoryUrl + releaseTagPathPrefix
	if !strings.HasPrefix(locationHeader, tagPrefix) {
		return "", fmt.Errorf("could not find latest release. redirected to %v", locationHeader)
	}

	return strings.TrimPrefix(locationHeader, tagPrefix), nil
}

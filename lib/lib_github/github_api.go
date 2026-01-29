package lib_github

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/buger/jsonparser"
	"github.com/friendsofgo/errors"
	"github.com/go-resty/resty/v2"
)

var ErrGithubNotFound = errors.New("github contents not found")

func GithubListContents(client *resty.Client, bearer string, owner, repo, path string, ref *string) ([]GithubContentItem, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", url.PathEscape(owner), url.PathEscape(repo), path)
	if ref != nil {
		apiURL += "?ref=" + url.QueryEscape(*ref)
	}

	res, err := client.R().
		SetHeader("Accept", "application/vnd.github+json").
		SetHeader("X-GitHub-Api-Version", "2022-11-28").
		SetAuthToken(bearer).
		Get(apiURL)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list items from Github")
	}

	switch res.StatusCode() {
	case 200:
		var items []GithubContentItem
		if err := json.Unmarshal(res.Body(), &items); err != nil {
			return nil, errors.Wrap(err, "failed to parse response from Github")
		}

		return items, nil
	case 404:
		return nil, errors.Wrap(ErrGithubNotFound, res.String())
	default:
		return nil, fmt.Errorf("expected 200 status code, got %d: %s", res.StatusCode(), res.String())
	}
}

type GithubContentItem struct {
	Links       GithubContentItemLinks `json:"_links"`
	Content     *string                `json:"content,omitempty"`
	DownloadURL *string                `json:"download_url"`
	Encoding    *string                `json:"encoding,omitempty"`
	Entries     []Entry                `json:"entries,omitempty"`
	GitURL      *string                `json:"git_url"`
	HTMLURL     *string                `json:"html_url"`
	Name        string                 `json:"name"`
	Path        string                 `json:"path"`
	SHA         string                 `json:"sha"`
	Size        int64                  `json:"size"`
	Type        string                 `json:"type"`
	URL         string                 `json:"url"`
}

type Entry struct {
	Links       EntryLinks `json:"_links"`
	DownloadURL *string    `json:"download_url"`
	GitURL      *string    `json:"git_url"`
	HTMLURL     *string    `json:"html_url"`
	Name        string     `json:"name"`
	Path        string     `json:"path"`
	SHA         string     `json:"sha"`
	Size        int64      `json:"size"`
	Type        string     `json:"type"`
	URL         string     `json:"url"`
}

type EntryLinks struct {
	Git  *string `json:"git"`
	HTML *string `json:"html"`
	Self string  `json:"self"`
}

type GithubContentItemLinks struct {
	Git  *string `json:"git"`
	HTML *string `json:"html"`
	Self string  `json:"self"`
}

func GithubListTree(client *resty.Client, bearer string, owner, repo, treeSha string, recursive bool) (*GithubTree, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/git/trees/%s", url.PathEscape(owner), url.PathEscape(repo), treeSha)
	if recursive {
		apiURL += "?recursive=true"
	}

	res, err := client.R().
		SetHeader("Accept", "application/vnd.github+json").
		SetHeader("X-GitHub-Api-Version", "2022-11-28").
		SetAuthToken(bearer).
		Get(apiURL)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list tree from Github")
	}

	switch res.StatusCode() {
	case 200:
		var tree GithubTree
		if err := json.Unmarshal(res.Body(), &tree); err != nil {
			return nil, errors.Wrap(err, "failed to parse response from Github")
		}

		return &tree, nil
	case 404:
		return nil, errors.Wrap(ErrGithubNotFound, res.String())
	default:
		return nil, fmt.Errorf("expected 200 status code, got %d: %s", res.StatusCode(), res.String())
	}
}

type GithubTree struct {
	SHA string `json:"sha"`
	// Objects specifying a tree structure
	Tree      []Tree  `json:"tree"`
	Truncated bool    `json:"truncated"`
	URL       *string `json:"url,omitempty"`
}

type Tree struct {
	Mode string  `json:"mode"`
	Path string  `json:"path"`
	SHA  string  `json:"sha"`
	Size *int64  `json:"size,omitempty"`
	Type string  `json:"type"`
	URL  *string `json:"url,omitempty"`
}

func GithubStartDeviceFlow(client *resty.Client, clientId string, redirectUri *string) (*DeviceStartResponse, error) {
	req := client.R().
		SetHeader("Accept", "application/json").
		SetQueryParam("client_id", clientId)

	if redirectUri != nil {
		req = req.SetQueryParam("redirect_uri", *redirectUri)
	}

	res, err := req.Post("https://github.com/login/device/code")
	if err != nil {
		return nil, errors.Wrap(err, "request failed")
	}

	if res.StatusCode() != 200 {
		return nil, errors.Errorf("expected 200 status code, got %d: %s", res.StatusCode(), res.String())
	}

	var data DeviceStartResponse
	if err := json.Unmarshal(res.Body(), &data); err != nil {
		return nil, errors.Wrap(err, "failed to parse response")
	}

	return &data, nil
}

type DeviceStartResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

var (
	ErrGithubAuthorizationPending = errors.New("github authorization pending")
	ErrGithubSlowDown             = errors.New("github slow down")
	ErrGithubExpiredToken         = errors.New("github expired token")
	ErrAccessDenied               = errors.New("github access denied")
)

func GithubGetAccessToken(client *resty.Client, clientId string, deviceCode string) (*GithubAccessToken, error) {
	res, err := client.R().
		SetHeader("Accept", "application/json").
		SetQueryParam("client_id", clientId).
		SetQueryParam("device_code", deviceCode).
		SetQueryParam("grant_type", "urn:ietf:params:oauth:grant-type:device_code").
		Post("https://github.com/login/oauth/access_token")
	if err != nil {
		return nil, errors.Wrap(err, "request failed")
	}

	if res.StatusCode() != 200 {
		return nil, errors.Errorf("expected 200 status code, got %d: %s", res.StatusCode(), res.String())
	}

	body := res.Body()

	errStr, err := jsonparser.GetString(body, "error")
	if err != nil {
		switch {
		case errors.Is(err, jsonparser.KeyPathNotFoundError):
		// expected, normal response
		default:
			return nil, errors.Wrap(err, "failed to parse response")
		}
	} else {
		switch errStr {
		case "authorization_pending":
			return nil, ErrGithubAuthorizationPending
		case "slow_down":
			return nil, ErrGithubSlowDown
		case "expired_token":
			return nil, ErrGithubExpiredToken
		case "access_denied":
			return nil, ErrAccessDenied
		default:
			return nil, fmt.Errorf("unexpected error: %s", errStr)
		}
	}

	var data GithubAccessToken
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, errors.Wrap(err, "failed to parse response")
	}

	return &data, nil
}

type GithubAccessToken struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
}

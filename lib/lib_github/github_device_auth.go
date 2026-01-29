package lib_github

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/fatih/color"
	"github.com/friendsofgo/errors"
	"github.com/go-resty/resty/v2"
	"github.com/schoolyear/avd-cli/static"
	"github.com/zalando/go-keyring"
)

func getGithubKeyringKeyName(clientId string) string {
	return "gh-device-" + clientId
}

func GithubDeviceFlow(client *resty.Client, clientId string, checkKeyringCache, writeToKeyring bool, reason string) (token string, err error) {
	keyringKeyName := getGithubKeyringKeyName(clientId)

	if checkKeyringCache {
		value, err := keyring.Get(static.KeyringServiceName, keyringKeyName)
		if err != nil {
			if !errors.Is(err, keyring.ErrNotFound) {
				return "", errors.Wrap(err, "failed to get github secret from local keyring")
			}
		} else {
			var keyringValue githubTokenKeyringValue
			if err := json.Unmarshal([]byte(value), &keyringValue); err != nil {
				color.HiRed("ERROR: Failed to parse github secret from local keyring")
			} else if time.Now().Before(keyringValue.ExpiresAt) {
				return keyringValue.AccessToken, nil
			}
		}
	}

	startRes, err := GithubStartDeviceFlow(client, clientId, nil)
	if err != nil {
		return "", errors.Wrap(err, "failed to start device flow")
	}

	color.Yellow("Log into GitHub: %s", reason)
	color.Yellow("    1. Open: %s", startRes.VerificationURI)
	color.Yellow("    2. Enter: %s", startRes.UserCode)
	color.Yellow(`    3. Click "Authorize"`)
	color.Yellow("    4. Return here")
	fmt.Printf("Waiting...")

	for {
		tokenRes, err := GithubGetAccessToken(client, clientId, startRes.DeviceCode)
		if err != nil {
			switch {
			case errors.Is(err, ErrGithubAuthorizationPending):
				fmt.Printf(".")
			case errors.Is(err, ErrGithubSlowDown):
				fmt.Printf("x")
				time.Sleep(time.Duration(startRes.Interval) * time.Second)
			case errors.Is(err, ErrGithubExpiredToken):
				return "", errors.Wrap(err, "github authorization took too long")
			case errors.Is(err, ErrAccessDenied):
				return "", errors.Wrap(err, "user denied access to GitHub")
			default:
				return "", errors.Wrap(err, "failed to get github access token")
			}

			time.Sleep(time.Duration(startRes.Interval) * time.Second)
		} else {
			token = tokenRes.AccessToken
			break
		}
	}

	if writeToKeyring {
		valueJson, err := json.Marshal(githubTokenKeyringValue{
			AccessToken: token,
			ExpiresAt:   time.Now().Add(4 * time.Hour),
		})
		if err != nil {
			return "", errors.Wrap(err, "failed to serialize github access token")
		}

		if err := keyring.Set(static.KeyringServiceName, keyringKeyName, string(valueJson)); err != nil {
			return "", errors.Wrap(err, "failed to write github secret to local keyring")
		}
	}

	fmt.Println()
	fmt.Println()

	return token, nil
}

type githubTokenKeyringValue struct {
	AccessToken string    `json:"access_token"`
	ExpiresAt   time.Time `json:"expires_at"`
}

package lib

import (
	"context"
	"time"

	"github.com/friendsofgo/errors"
	"github.com/go-resty/resty/v2"
)

const (
	downloadTimeout    = 2 * time.Second
	downloadRetryCount = 2
)

func DownloadFile(ctx context.Context, url string) ([]byte, error) {
	res, err := resty.New().
		SetTimeout(downloadTimeout).
		SetRetryCount(downloadRetryCount).
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

package openrouter

import (
	"errors"
	"fmt"

	gopenrouter "github.com/revrost/go-openrouter"
)

var (
	ErrMissingBaseURL  = errors.New("openrouter base url is required")
	ErrMissingAPIToken = errors.New("openrouter api token is required")

	ErrBadRequest   = errors.New("openrouter request is invalid")
	ErrUnauthorized = errors.New("openrouter credentials are invalid")
	ErrNotFound     = errors.New("openrouter resource not found")
	ErrRateLimited  = errors.New("openrouter rate limit exceeded")
	ErrUnavailable  = errors.New("openrouter is unavailable")

	ErrModelRequired = errors.New("openrouter model is required")
	ErrModelNotFound = errors.New("openrouter model not found")
)

func wrapError(err error) error {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, ErrMissingBaseURL),
		errors.Is(err, ErrMissingAPIToken),
		errors.Is(err, ErrBadRequest),
		errors.Is(err, ErrUnauthorized),
		errors.Is(err, ErrNotFound),
		errors.Is(err, ErrRateLimited),
		errors.Is(err, ErrUnavailable),
		errors.Is(err, ErrModelRequired),
		errors.Is(err, ErrModelNotFound):
		return err
	}

	var apiErr *gopenrouter.APIError
	if errors.As(err, &apiErr) {
		return wrapStatusError(apiErr.HTTPStatusCode, err)
	}

	var reqErr *gopenrouter.RequestError
	if errors.As(err, &reqErr) {
		return wrapStatusError(reqErr.HTTPStatusCode, err)
	}

	return err
}

func wrapStatusError(code int, err error) error {
	switch code {
	case 400:
		return fmt.Errorf("%w: %v", ErrBadRequest, err)
	case 401, 403:
		return fmt.Errorf("%w: %v", ErrUnauthorized, err)
	case 404:
		return fmt.Errorf("%w: %v", ErrNotFound, err)
	case 408, 429:
		return fmt.Errorf("%w: %v", ErrRateLimited, err)
	case 500, 502, 503, 504:
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	default:
		return err
	}
}

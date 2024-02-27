package apigen

import "github.com/pkg/errors"

type SpecResponse interface {
	Status() string
	StatusCode() int
}

func ExpectStatusCodeWithMessage(res SpecResponse, statusCode int, msg string, args ...string) error {
	if res.StatusCode() != statusCode {
		return errors.Errorf("expected status code %d but got %d, message: ", statusCode, res.StatusCode())
	}
	return nil
}

func ExpectStatusCodeWithError(res SpecResponse, statusCode int, err error) error {
	if res.StatusCode() != statusCode {
		return errors.Wrapf(err, "expected status code %d but got %d, message: ", statusCode, res.StatusCode())
	}
	return nil
}

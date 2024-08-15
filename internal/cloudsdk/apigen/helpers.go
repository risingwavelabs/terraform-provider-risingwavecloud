package apigen

import (
	"fmt"

	"github.com/pkg/errors"
)

type SpecResponse interface {
	Status() string
	StatusCode() int
}

func ExpectStatusCodeWithMessage(res SpecResponse, statusCode int, message ...string) error {
	if res.StatusCode() != statusCode {
		content := fmt.Sprintf("expected status code %d but got %d", statusCode, res.StatusCode())
		if len(message) > 0 {
			content = fmt.Sprintf("%s, message: %s", content, message[0])
		}
		return errors.New(content)
	}
	return nil
}

func ExpectStatusCodeWithError(res SpecResponse, statusCode int, err error) error {
	if res.StatusCode() != statusCode {
		return errors.Wrapf(err, "expected status code %d but got %d, error: %s", statusCode, res.StatusCode(), err.Error())
	}
	return nil
}

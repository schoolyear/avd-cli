package lib

import (
	stdErr "errors"
	"github.com/friendsofgo/errors"
	avdimagetypes "github.com/schoolyear/avd-image-types"
)

func ValidateAVDImageType(definition string, payload []byte) error {
	validationResult, err := avdimagetypes.ValidateDefinition(definition, payload)
	if err != nil {
		return errors.Wrap(err, "failed to validate")
	} else if !validationResult.Valid() {
		resultErrors := validationResult.Errors()
		validationErrors := make([]error, len(resultErrors))
		for i, validationError := range resultErrors {
			validationErrors[i] = errors.New(validationError.String())
		}

		return stdErr.Join(validationErrors...)
	}
	return nil
}

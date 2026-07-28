package sql

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
)

var (
	ErrNotFound    = errors.New("not found")
	ErrConflict    = errors.New("conflict")
	ErrInvalid     = errors.New("invalid data")
	ErrUnavailable = errors.New("storage unavailable")
)

func TranslateError(err error) error {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, ErrNotFound),
		errors.Is(err, ErrConflict),
		errors.Is(err, ErrInvalid),
		errors.Is(err, ErrUnavailable):
		return err
	case errors.Is(err, gorm.ErrRecordNotFound):
		return fmt.Errorf("%w: %w", ErrNotFound, err)
	case errors.Is(err, gorm.ErrDuplicatedKey):
		return fmt.Errorf("%w: %w", ErrConflict, err)
	case errors.Is(err, gorm.ErrForeignKeyViolated),
		errors.Is(err, gorm.ErrCheckConstraintViolated),
		errors.Is(err, gorm.ErrInvalidData):
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	case errors.Is(err, gorm.ErrInvalidDB):
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	default:
		return err
	}
}

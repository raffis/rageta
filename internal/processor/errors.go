package processor

import "errors"

func AbortOnError(err error) bool {
	switch {
	case errors.Is(err, ErrAllowFailure):
		return false
	case errors.Is(err, ErrConditionFalse):
		return false
	case errors.Is(err, ErrSkipDone):
		return false
	case err != nil:
		return true
	default:
		return false
	}
}

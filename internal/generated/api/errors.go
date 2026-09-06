package api

import "errors"

// ErrStrictOperationNotImplemented indicates that an OpenAPI operation has no milestone implementation yet.
var ErrStrictOperationNotImplemented = errors.New("OpenAPI operation is not implemented")

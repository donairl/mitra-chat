package ws

import "errors"

// ErrForbidden is returned when a user acts on a resource they do not own.
var ErrForbidden = errors.New("forbidden")

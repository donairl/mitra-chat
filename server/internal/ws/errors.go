package ws

import "errors"

// ErrForbidden is returned when a user may see a resource but not change it.
var ErrForbidden = errors.New("forbidden")

// ErrNotFound is returned when a resource is missing or hidden from the user.
var ErrNotFound = errors.New("not found")

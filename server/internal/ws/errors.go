package ws

import "errors"

// ErrForbidden is returned when a user may see a resource but not change it.
var ErrForbidden = errors.New("forbidden")

// ErrNotFound is returned when a resource is missing or hidden from the user.
var ErrNotFound = errors.New("not found")

// ErrEmptyContent is returned when a message would have no text (and, for a
// new message, no attachments).
var ErrEmptyContent = errors.New("empty content")

// ErrContentTooLong is returned when message text exceeds MaxContentLen.
var ErrContentTooLong = errors.New("content too long")

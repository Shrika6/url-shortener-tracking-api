package services

import "errors"

var (
	ErrInvalidURL      = errors.New("invalid url")
	ErrShortCodeNotFound = errors.New("short code not found")
)

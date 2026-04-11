package services

import "errors"

var (
	ErrInvalidURL        = errors.New("invalid url")
	ErrInvalidCustomCode = errors.New("invalid custom code")
	ErrReservedShortCode = errors.New("reserved short code")
	ErrCustomCodeConflict = errors.New("custom code already exists")
	ErrShortCodeNotFound = errors.New("short code not found")
)

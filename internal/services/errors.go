package services

import "errors"

var (
	ErrInvalidURL        = errors.New("invalid url")
	ErrInvalidCustomCode = errors.New("invalid custom code")
	ErrInvalidExpiry     = errors.New("invalid expiry")
	ErrInvalidStatsQuery = errors.New("invalid stats query")
	ErrReservedShortCode = errors.New("reserved short code")
	ErrCustomCodeConflict = errors.New("custom code already exists")
	ErrShortCodeExpired  = errors.New("short code expired")
	ErrShortCodeNotFound = errors.New("short code not found")
)

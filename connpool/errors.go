package connpool

import "errors"

// Pre-defined errors for connection pool operations
var (
	ErrPoolClosed    = errors.New("connection pool is closed")
	ErrPoolExhausted = errors.New("connection pool exhausted")
)

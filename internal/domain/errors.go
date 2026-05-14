package domain

import "errors"

var (
	ErrNodeNotFound      = errors.New("node not found")
	ErrCommandDisabled   = errors.New("command not enabled for this node")
	ErrInvalidTarget     = errors.New("invalid target")
	ErrTimeout           = errors.New("command timeout")
	ErrCircuitOpen       = errors.New("circuit breaker open")
	ErrQueryPoolFull     = errors.New("query pool full")
	ErrRateLimited       = errors.New("rate limit exceeded")
	ErrNodeUnavailable   = errors.New("node unavailable")
	ErrUnauthorized      = errors.New("unauthorized")
	ErrNoRouteFound      = errors.New("no route found")
	ErrParseFailure      = errors.New("parse failure")
	ErrInvalidNodeConfig = errors.New("invalid node configuration")
	ErrNotFound          = errors.New("not found")
	ErrCannotDeleteSelf  = errors.New("cannot delete own account")
)

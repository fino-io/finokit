package messaging

import "errors"

var (
	ErrNilConfig       = errors.New("messaging: config is nil")
	ErrNilClient       = errors.New("messaging: client is nil")
	ErrNilQueue        = errors.New("messaging: queue is nil")
	ErrNilMessage      = errors.New("messaging: message is nil")
	ErrNilSubscription = errors.New("messaging: subscription is nil")
	ErrNilHandler      = errors.New("messaging: message handler is nil")
	ErrEmptyTopic      = errors.New("messaging: topic is empty")
	ErrUnknownEndpoint = errors.New("messaging: endpoint is not registered")
)

package messaging

import "io"

//go:generate mockgen -destination=mocks/queue.go -package=mocks . Queue
type Queue interface {
	Publisher
	Subscriber

	io.Closer
}

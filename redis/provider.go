package redis

import (
	"context"
	"io"

	goredis "github.com/redis/go-redis/v9"
)

var _ Provider = (*Service)(nil)

type Provider interface {
	Raw() goredis.UniversalClient
	Ping(ctx context.Context) error

	io.Closer
}

func (s *Service) Raw() goredis.UniversalClient {
	if s == nil {
		return nil
	}
	return s.raw
}

func (s *Service) Ping(ctx context.Context) error {
	if s == nil || s.raw == nil {
		return ErrNilService
	}
	return s.raw.Ping(ctx).Err()
}

func (s *Service) Close() error {
	if s == nil || s.raw == nil {
		return nil
	}

	s.closeOnce.Do(func() {
		s.closeErr = s.raw.Close()
	})
	return s.closeErr
}

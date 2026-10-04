package logs

import (
	"context"

	corelogs "github.com/fino-io/core/go/logs"
)

// Service keeps trace enrichment when deriving a logging service.
type Service struct {
	*corelogs.Service
}

func NewService(logger Logger) *Service {
	return &Service{Service: corelogs.NewService(withTrace(logger))}
}

// Ctx returns the default logging service bound to ctx.
func Ctx(ctx context.Context) *Service {
	return NewService(corelogs.DefaultLogger()).WithContext(ctx)
}

func (s *Service) WithContext(ctx context.Context) *Service {
	if s == nil {
		return NewService(nil).WithContext(ctx)
	}
	return &Service{Service: s.Service.WithContext(ctx)}
}

func (s *Service) WithLogger(logger Logger) *Service {
	if s == nil {
		return NewService(logger)
	}
	return &Service{Service: s.Service.WithLogger(withTrace(logger))}
}

func defaultService() *corelogs.Service {
	return corelogs.NewServiceWithCallerSkip(DefaultLogger(), 1)
}

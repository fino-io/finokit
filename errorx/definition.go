package errorx

import (
	"net/http"
	"strings"
)

const (
	DefaultMessage    = "Service Internal Error"
	DefaultCountInSLA = true
	DefaultHTTPStatus = http.StatusInternalServerError
)

// Definition describes one stable business error code.
type Definition struct {
	Code       int32
	Message    string
	CountInSLA bool
	HTTPStatus int
}

type DefineOption func(*Definition)

func Define(code int32, message string, opts ...DefineOption) Definition {
	def := Definition{
		Code:       code,
		Message:    message,
		CountInSLA: DefaultCountInSLA,
	}
	for _, opt := range opts {
		opt(&def)
	}
	return def
}

func CountInSLA(countInSLA bool) DefineOption {
	return func(def *Definition) {
		def.CountInSLA = countInSLA
	}
}

func HTTPStatus(status int) DefineOption {
	return func(def *Definition) {
		def.HTTPStatus = status
	}
}

func (def Definition) New(opts ...Option) error {
	err := newError(def, nil)
	applyOptions(err, opts...)
	return err
}

func (def Definition) Wrap(cause error, opts ...Option) error {
	if cause == nil {
		return nil
	}
	err := newError(def, cause)
	applyOptions(err, opts...)
	return err
}

func (def Definition) Is(err error) bool {
	return Is(err, def.Code)
}

func (def Definition) StatusCode() int {
	if def.HTTPStatus < http.StatusContinue || def.HTTPStatus > 599 {
		return DefaultHTTPStatus
	}
	return def.HTTPStatus
}

func (def Definition) normalized() Definition {
	if strings.TrimSpace(def.Message) == "" {
		def.Message = DefaultMessage
	}
	def.HTTPStatus = def.StatusCode()
	return def
}

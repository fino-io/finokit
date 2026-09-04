package logs

import (
	"context"

	corelogs "github.com/fino-io/core/go/logs"
	"github.com/fino-io/finokit/config"
)

type Config = corelogs.Config
type FileConfig = corelogs.FileConfig
type Level = corelogs.Level
type Logger = corelogs.Logger
type Field = corelogs.Field
type Entry = corelogs.Entry
type Service = corelogs.Service

const (
	DebugLevel  = corelogs.DebugLevel
	InfoLevel   = corelogs.InfoLevel
	WarnLevel   = corelogs.WarnLevel
	ErrorLevel  = corelogs.ErrorLevel
	DPanicLevel = corelogs.DPanicLevel
	PanicLevel  = corelogs.PanicLevel
	FatalLevel  = corelogs.FatalLevel
)

func NewDefaultConfig() *Config {
	return corelogs.NewDefaultConfig()
}

func NewLoggerWith(cfg *Config) Logger {
	return withTrace(corelogs.NewLoggerWith(cfg))
}

// NewConfig loads the logs section from the current default config and
// replaces the package default logger.
func NewConfig() error {
	cfg := NewDefaultConfig()
	if err := config.ScanFrom(cfg, "logs"); err != nil {
		return err
	}

	SetLogger(NewLoggerWith(cfg))

	return nil
}

func DefaultLogger() Logger {
	return withTrace(corelogs.DefaultLogger())
}

func SetLogger(logger Logger) {
	corelogs.SetLogger(withTrace(logger))
}

func SetLogLevel(level Level) {
	corelogs.SetLogLevel(level)
}

func Debug(args ...interface{}) {
	defaultService().Debug(args...)
}

func Info(args ...interface{}) {
	defaultService().Info(args...)
}

func Warn(args ...interface{}) {
	defaultService().Warn(args...)
}

func Error(args ...interface{}) {
	defaultService().Error(args...)
}

func Fatal(args ...interface{}) {
	defaultService().Fatal(args...)
}

func Debugf(template string, args ...interface{}) {
	defaultService().Debugf(template, args...)
}

func Infof(template string, args ...interface{}) {
	defaultService().Infof(template, args...)
}

func Warnf(template string, args ...interface{}) {
	defaultService().Warnf(template, args...)
}

func Errorf(template string, args ...interface{}) {
	defaultService().Errorf(template, args...)
}

func Fatalf(template string, args ...interface{}) {
	defaultService().Fatalf(template, args...)
}

func Debugw(msg string, keysAndValues ...interface{}) {
	defaultService().Debugw(msg, keysAndValues...)
}

func Infow(msg string, keysAndValues ...interface{}) {
	defaultService().Infow(msg, keysAndValues...)
}

func Warnw(msg string, keysAndValues ...interface{}) {
	defaultService().Warnw(msg, keysAndValues...)
}

func Errorw(msg string, keysAndValues ...interface{}) {
	defaultService().Errorw(msg, keysAndValues...)
}

func Fatalw(msg string, keysAndValues ...interface{}) {
	defaultService().Fatalw(msg, keysAndValues...)
}

func NewError(args ...interface{}) error {
	return defaultService().NewError(args...)
}

func NewErrorf(template string, args ...interface{}) error {
	return defaultService().NewErrorf(template, args...)
}

func NewErrorw(msg string, keysAndValues ...interface{}) error {
	return defaultService().NewErrorw(msg, keysAndValues...)
}

func WithFields(ctx context.Context, fields ...Field) context.Context {
	return corelogs.WithFields(ctx, fields...)
}

// Ctx returns the default logging service bound to ctx.
func Ctx(ctx context.Context) *Service {
	return NewService(corelogs.DefaultLogger()).WithContext(ctx)
}

func NewService(logger Logger) *Service {
	return corelogs.NewService(withTrace(logger))
}

func defaultService() *corelogs.Service {
	return corelogs.NewServiceWithCallerSkip(withTrace(corelogs.DefaultLogger()), 1)
}

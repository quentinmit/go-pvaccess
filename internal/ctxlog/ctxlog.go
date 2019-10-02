// ctxlog provides context-aware logging.
// Loosely based on https://github.com/containerd/containerd/blob/master/log/context.go
package ctxlog

import (
	"context"

	"github.com/sirupsen/logrus"
)

var (
	// L is an alias for Logger.
	L = Logger
)

type (
	loggerKey struct{}
)

// WithLogger returns a new context with the provided logger. Use in
// combination with logger.WithField(s) for great effect.
func WithLogger(ctx context.Context, logger *logrus.Entry) context.Context {
	return context.WithValue(ctx, loggerKey{}, logger)
}

// WithField returns a new context with the provided field set in the existing logger.
func WithField(ctx context.Context, key string, value interface{}) context.Context {
	return WithLogger(ctx, Logger(ctx).WithField(key, value))
}

// WithFields returns a new context with the provided fields set in the existing logger.
func WithFields(ctx context.Context, fields logrus.Fields) context.Context {
	return WithLogger(ctx, Logger(ctx).WithFields(fields))
}

type Fields = logrus.Fields

// Logger retrieves the current logger from the context. If no logger is
// available, the default logger is returned.
func Logger(ctx context.Context) *logrus.Entry {
	logger := ctx.Value(loggerKey{})

	if logger == nil {
		return logrus.NewEntry(logrus.StandardLogger())
	}

	return logger.(*logrus.Entry)
}

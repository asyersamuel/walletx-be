package logger

import (
	"github.com/sirupsen/logrus"
)

// Logger is the minimal logging interface used across the application.
// It abstracts the concrete logging library so business modules do not
// depend directly on logrus.
type Logger interface {
	Info(msg string)
	Warn(msg string)
	Error(msg string)
	Debug(msg string)
	WithField(key string, value interface{}) Logger
	WithError(err error) Logger
	WithFields(fields map[string]interface{}) Logger
}

type logrusAdapter struct {
	entry *logrus.Entry
}

func NewLogger() *logrusAdapter {
	return &logrusAdapter{entry: logrus.NewEntry(logrus.StandardLogger())}
}

func (l *logrusAdapter) Info(msg string) {
	l.entry.Info(msg)
}

func (l *logrusAdapter) Warn(msg string) {
	l.entry.Warn(msg)
}

func (l *logrusAdapter) Error(msg string) {
	l.entry.Error(msg)
}

func (l *logrusAdapter) Debug(msg string) {
	l.entry.Debug(msg)
}

func (l *logrusAdapter) WithField(key string, value interface{}) Logger {
	return &logrusAdapter{entry: l.entry.WithField(key, value)}
}

func (l *logrusAdapter) WithError(err error) Logger {
	return &logrusAdapter{entry: l.entry.WithError(err)}
}

func (l *logrusAdapter) WithFields(fields map[string]interface{}) Logger {
	return &logrusAdapter{entry: l.entry.WithFields(fields)}
}

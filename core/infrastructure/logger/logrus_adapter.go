package logger

import (
	"walletx-be/core/ports"

	"github.com/sirupsen/logrus"
)

type logrusAdapter struct {
	entry *logrus.Entry
}

type Logger = ports.Logger

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

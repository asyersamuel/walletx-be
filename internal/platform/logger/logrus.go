package logger

import (
	"os"

	"github.com/sirupsen/logrus"
)

// Logger is the minimal logging interface used across the application.
type Logger interface {
	Info(msg string)
	Warn(msg string)
	Error(msg string)
	Debug(msg string)
	WithField(key string, value interface{}) Logger
	WithError(err error) Logger
	WithFields(fields map[string]interface{}) Logger
}

func init() {
	Configure()
}

// Configure sets the process-wide logging defaults used by the adapter.
func Configure() {
	logrus.SetFormatter(&logrus.JSONFormatter{})
	logrus.SetOutput(os.Stdout)
	logrus.SetLevel(logrus.InfoLevel)
}

type logrusAdapter struct {
	entry *logrus.Entry
}

func NewLogger() Logger {
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

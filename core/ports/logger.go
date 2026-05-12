package ports

type Logger interface {
	Info(msg string)
	Warn(msg string)
	Error(msg string)
	Debug(msg string)
	WithField(key string, value interface{}) Logger
	WithError(err error) Logger
	WithFields(fields map[string]interface{}) Logger
}

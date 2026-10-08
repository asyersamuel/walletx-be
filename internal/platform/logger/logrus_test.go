package logger

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func TestJSONLoggerPreservesRawErrorDetailWithoutTruncation(t *testing.T) {
	var output bytes.Buffer
	adapter := logrus.New()
	adapter.SetOutput(&output)
	adapter.SetFormatter(&logrus.JSONFormatter{})
	appLogger := &logrusAdapter{entry: logrus.NewEntry(adapter)}
	detail := strings.Repeat("SDK error chain; ", 1000) + "\nread tcp: context deadline exceeded"
	appLogger.WithFields(map[string]interface{}{"provider_raw_error_detail": detail}).Error("receipt processing could not complete")
	var event map[string]interface{}
	require.NoError(t, json.Unmarshal(output.Bytes(), &event))
	require.Equal(t, detail, event["provider_raw_error_detail"])
}

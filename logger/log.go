package logger

import (
	"io"
	"runtime"
	"strings"

	"github.com/sirupsen/logrus"
)

// Minimally Viable Logger

// This Package exists because I couldn't decide on a logging framework that didn't infuriate me.
// So this is simple place to make a better decision later about logging frameworks. I only care about
// the interface, not the implementation. Smarter people do that well.

func SetDebug() {
	logrus.SetLevel(logrus.DebugLevel)
}

func SetError() {
	logrus.SetLevel(logrus.ErrorLevel)
}

func Package() Logger {
	_, p, _, _ := runtime.Caller(1)
	_, suffix, _ := strings.Cut(p, "obot")
	i := strings.LastIndex(suffix, "/")
	if i > 0 {
		return New(suffix[:i])
	}
	return New(p)
}

func NewWithFields(fields logrus.Fields) Logger {
	return Logger{
		log:    logrus.StandardLogger(),
		fields: fields,
	}
}

func New(name string) Logger {
	var fields logrus.Fields
	if name != "" {
		fields = logrus.Fields{
			"logger": name,
		}
	}
	return NewWithFields(fields)
}

func SetOutput(out io.Writer) {
	logrus.SetOutput(out)
}

type Logger struct {
	log    *logrus.Logger
	fields logrus.Fields
}

func (l *Logger) FieldsMap(kv map[string]any) *Logger {
	newFields := map[string]any{}
	for k, v := range l.fields {
		newFields[k] = v
	}
	for k, v := range kv {
		newFields[k] = v
	}
	return &Logger{
		log:    l.log,
		fields: newFields,
	}
}

func (l *Logger) Fields(kv ...any) *Logger {
	newFields := map[string]any{}
	for k, v := range l.fields {
		newFields[k] = v
	}
	for i, v := range kv {
		if i%2 == 1 {
			newFields[kv[i-1].(string)] = v
		}
	}
	return &Logger{
		log:    l.log,
		fields: newFields,
	}
}

func (l *Logger) Infof(msg string, args ...any) {
	l.log.WithFields(l.fields).Infof(msg, args...)
}

func (l *Logger) Errorf(msg string, args ...any) {
	l.log.WithFields(l.fields).Errorf(msg, args...)
}

func (l *Logger) Tracef(msg string, args ...any) {
	l.log.WithFields(l.fields).Tracef(msg, args...)
}

func (l *Logger) Warnf(msg string, args ...any) {
	l.log.WithFields(l.fields).Warnf(msg, args...)
}

func (l *Logger) IsDebug() bool {
	return l.log.IsLevelEnabled(logrus.DebugLevel)
}

func (l *Logger) Debugf(msg string, args ...any) {
	l.log.WithFields(l.fields).Debugf(msg, args...)
}

func (l *Logger) Fatalf(msg string, args ...any) {
	l.log.WithFields(l.fields).Fatalf(msg, args...)
}

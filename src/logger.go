package vncclient

import (
	"log"
	"os"
)

type Logger interface {
	Fatal(string, ...interface{})
	Warn(string, ...interface{})
	Debug(string, ...interface{})
}

var logger *Logger

func SetDefaultLogger() {
	logger = &BaseLogger{
		Logger: log.New(os.Stderr, "", log.Lmicroseconds|log.Ldate|log.Ltime),
	}
}

type BaseLogger struct {
	Logger *log.Logger
}

func SetLogger(l *Logger) {
	logger = l
}

func (bl BaseLogger) Fatal(format string, v ...interface{}) {
	bl.Logger.Printf("FATAL: "+format, v...)
}

func (bl BaseLogger) Warn(format string, v ...interface{}) {
	bl.Logger.Printf("WARN: "+format, v...)
}

func (bl BaseLogger) Debug(format string, v ...interface{}) {
	bl.Logger.Printf("DEBUG: "+format, v...)
}

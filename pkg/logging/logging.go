package logging

import (
	"log"
	"os"
)

// Logger defines the interface for VARA's unified logging abstraction.
type Logger interface {
	Info(format string, args ...any)
	Warn(format string, args ...any)
	Error(format string, args ...any)
	Debug(format string, args ...any)
}

type stdLogger struct {
	*log.Logger
}

func (l *stdLogger) Info(format string, args ...any)  { l.Printf("[INFO] "+format, args...) }
func (l *stdLogger) Warn(format string, args ...any)  { l.Printf("[WARN] "+format, args...) }
func (l *stdLogger) Error(format string, args ...any) { l.Printf("[ERROR] "+format, args...) }
func (l *stdLogger) Debug(format string, args ...any) { l.Printf("[DEBUG] "+format, args...) }

// Default returns a standard logger pointing to stderr.
func Default() Logger {
	return &stdLogger{
		Logger: log.New(os.Stderr, "vara: ", log.LstdFlags),
	}
}

// Global logger instance.
var Global = Default()

func Info(format string, args ...any)  { Global.Info(format, args...) }
func Warn(format string, args ...any)  { Global.Warn(format, args...) }
func Error(format string, args ...any) { Global.Error(format, args...) }
func Debug(format string, args ...any) { Global.Debug(format, args...) }

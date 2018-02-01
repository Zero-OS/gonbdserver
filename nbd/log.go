package nbd

import (
	"fmt"
	"log"
)

// Logger defines a pragmatic Logger interface.
type Logger interface {
	// verbose messages targeted at the developer
	Debug(args ...interface{})
	Debugf(format string, args ...interface{})
	// info messages targeted at the user and developer
	Info(args ...interface{})
	Infof(format string, args ...interface{})
	// error messages targeted at the user, sysadmin and developer,
	// but mostly at the sysadmin
	Error(args ...interface{})
	Errorf(format string, args ...interface{})
	// a fatal message targeted at the user and developer
	// the program will exit as this message
	// this level shouldn't be used by libraries
	Fatal(args ...interface{})
	Fatalf(format string, args ...interface{})
}

// StandardLogger does no logging, and is used as the default logger,
// should no logger be given.
type StandardLogger struct{}

// Debug implements Logger.Debug
func (l *StandardLogger) Debug(args ...interface{}) {
	log.Print(append([]interface{}{"[DEBUG] "}, args...)...)
}

// Debugf implements Logger.Debugf
func (l *StandardLogger) Debugf(format string, args ...interface{}) {
	log.Printf("[DEBUG] "+format, args...)
}

// Info implements Logger.Info
func (l *StandardLogger) Info(args ...interface{}) {
	log.Print(append([]interface{}{"[INFO] "}, args...)...)
}

// Infof implements Logger.Infof
func (l *StandardLogger) Infof(format string, args ...interface{}) {
	log.Printf("[INFO] "+format, args...)
}

// Error implements Logger.Error
func (l *StandardLogger) Error(args ...interface{}) {
	log.Print(append([]interface{}{"[ERROR] "}, args...)...)
}

// Errorf implements Logger.Errorf
func (l *StandardLogger) Errorf(format string, args ...interface{}) {
	log.Printf("[ERROR] "+format, args...)
}

// Fatal implements Logger.Fatal
func (l *StandardLogger) Fatal(args ...interface{}) {
	log.Print(append([]interface{}{"[FATAL] "}, args...)...)
	panic(fmt.Sprint(args...))
}

// Fatalf implements Logger.Fatalf
func (l *StandardLogger) Fatalf(format string, args ...interface{}) {
	log.Printf("[ERROR] "+format, args...)
	panic(fmt.Sprint(args...))
}

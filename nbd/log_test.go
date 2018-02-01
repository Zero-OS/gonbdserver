package nbd

import (
	"fmt"
	"log"
)

// TestLogger does no logging, and is used as the default logger,
// should no logger be given.
type TestLogger struct{}

// Debug implements Logger.Debug
func (l *TestLogger) Debug(args ...interface{}) {
	log.Print(append([]interface{}{"[DEBUG] "}, args...)...)
}

// Debugf implements Logger.Debugf
func (l *TestLogger) Debugf(format string, args ...interface{}) {
	log.Printf("[DEBUG] "+format, args...)
}

// Info implements Logger.Info
func (l *TestLogger) Info(args ...interface{}) {
	log.Print(append([]interface{}{"[INFO] "}, args...)...)
}

// Infof implements Logger.Infof
func (l *TestLogger) Infof(format string, args ...interface{}) {
	log.Printf("[INFO] "+format, args...)
}

// Error implements Logger.Error
func (l *TestLogger) Error(args ...interface{}) {
	log.Print(append([]interface{}{"[ERROR] "}, args...)...)
}

// Errorf implements Logger.Errorf
func (l *TestLogger) Errorf(format string, args ...interface{}) {
	log.Printf("[ERROR] "+format, args...)
}

// Fatal implements Logger.Fatal
func (l *TestLogger) Fatal(args ...interface{}) {
	log.Print(append([]interface{}{"[FATAL] "}, args...)...)
	panic(fmt.Sprint(args...))
}

// Fatalf implements Logger.Fatalf
func (l *TestLogger) Fatalf(format string, args ...interface{}) {
	log.Printf("[ERROR] "+format, args...)
	panic(fmt.Sprint(args...))
}

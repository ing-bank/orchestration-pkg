package logging

// Logging is the build-in log library with the addition of log levels. Use SetLevel to
// initialize the log-level, and then Logf to log. For example:
//
//  SetLevel(logging.INFO)
//  Logf(logging.DEBUG, "This log is ignored because DEBUG < INFO\n")
//  Logf(logging.INFO, "My name is %s\n", name) // Will be logged

import (
	"errors"
	"log"
	"strings"
)

const (
	NOTSET   = 0
	DEBUG    = 10
	INFO     = 20
	WARNING  = 30
	ERROR    = 40
	CRITICAL = 50
)

var currentLogLevel = NOTSET

func SetLevel(logLevel int) {
	currentLogLevel = logLevel
}

func SetLevelFromString(logLevel string) {
	level, err := ParseStringLogLevel(logLevel)
	SetLevel(level)

	if err != nil {
		Logf(ERROR, "[logging](SetLevelFromString): %s\n", err.Error())
	}
}

func ParseStringLogLevel(logLevel string) (int, error) {
	switch strings.ToUpper(logLevel) {
	case "DEBUG":
		return DEBUG, nil
	case "INFO":
		return INFO, nil
	case "WARNING":
		return WARNING, nil
	case "ERROR":
		return ERROR, nil
	case "CRITICAL":
		return CRITICAL, nil
	default:
		return NOTSET, errors.New("invalid format")
	}
}

func ParseIntLogLevel(logLevel int) string {
	switch logLevel {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARNING:
		return "WARNING"
	case ERROR:
		return "ERROR"
	case CRITICAL:
		return "CRITICAL"
	default:
		return "NOTSET"
	}
}

func Logf(logLevel int, format string, params ...interface{}) {
	if logLevel >= currentLogLevel {

		preFormat := "[%s] "
		preFormatParams := []interface{}{
			ParseIntLogLevel(logLevel),
		}
		preFormatParams = append(preFormatParams, params...)

		log.Printf(preFormat+format, preFormatParams...)
	}
}

func Fatalf(format string, params ...interface{}) {
	Logf(CRITICAL, format, params...)
	panic("logging.Fatalf") // Show "nice" trace
}

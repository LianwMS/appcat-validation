package logger

import (
	"io"
	"log"
	"os"
	"sync"
)

var (
	logger  *log.Logger
	once    sync.Once
	logFile *os.File
)

// Init initializes the global logger with a custom log file name and multi-writer support.
func Init(logFileName string, writeToConsole bool) error {
	var err error
	once.Do(func() {
		// Open log file
		logFile, err = os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return
		}

		// Create MultiWriter: file + optional console
		var writers []io.Writer
		writers = append(writers, logFile)
		if writeToConsole {
			writers = append(writers, os.Stdout)
		}
		multiWriter := io.MultiWriter(writers...)

		// Initialize the logger
		logger = log.New(multiWriter, "[GLOBAL] ", log.Ldate|log.Ltime|log.Lshortfile)
	})
	return err
}

// Get returns the global logger
func Get() *log.Logger {
	if logger == nil {
		panic("logger not initialized: call logger.Init first")
	}
	return logger
}

// CloseLogFile closes the opened log file
func CloseLogFile() error {
	if logFile != nil {
		return logFile.Close()
	}
	return nil
}

package logging

import (
	"io"
	"log"
	"os"
)

var (
	// File is the verbose logger — writes everything to the log file.
	File = log.New(io.Discard, "", log.Ldate|log.Ltime|log.Lmicroseconds)

	// Console is the brief logger — writes key events to stderr.
	Console = log.New(os.Stderr, "", log.Ltime)
)

// Init sets up both loggers. Call once from main.
func Init(logFile io.Writer) {
	File = log.New(logFile, "", log.Ldate|log.Ltime|log.Lmicroseconds)
	Console = log.New(os.Stderr, "", log.Ltime)

	// Also redirect standard log to file (for any legacy log.Printf calls)
	log.SetOutput(logFile)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
}

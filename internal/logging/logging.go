package logging

import (
	"io"
	"log"
	"os"
)

var (
	// File is the verbose logger — writes everything to the log file.
	// Initialized to a discard logger so tests that skip Init() don't panic.
	File = log.New(io.Discard, "", 0)

	// Console is the brief logger — writes key events to stderr.
	// Initialized to a discard logger so tests that skip Init() don't panic.
	Console = log.New(io.Discard, "", 0)
)

// Init sets up both loggers. Call once from main.
func Init(logFile io.Writer) {
	File = log.New(logFile, "", log.Ldate|log.Ltime|log.Lmicroseconds)
	Console = log.New(os.Stderr, "", log.Ltime)

	// Also redirect standard log to file (for any legacy log.Printf calls)
	log.SetOutput(logFile)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
}

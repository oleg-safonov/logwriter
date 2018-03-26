# logwriter golang
[![Build Status](https://travis-ci.org/oleg-safonov/logwriter.svg?branch=master)](https://travis-ci.org/oleg-safonov/logwriter)[![Coverage Status](https://coveralls.io/repos/github/oleg-safonov/logwriter/badge.svg?branch=master)](https://coveralls.io/github/oleg-safonov/logwriter?branch=master)[![GoDoc](https://godoc.org/github.com/oleg-safonov/logwriter?status.svg)](https://godoc.org/github.com/oleg-safonov/logwriter)
Should your application crash if the disk with the log file is full?
Should the application slow down if writing to disk is slower than the appearance of new portions of logs?
Package logwriter provides a LogWriter type with a circular buffer for logs that are written to the output io.Writer whenever possible.
If the buffer overflows, the new record is skipped and one of SkipHandler or WriteErrorHandler is called.

Package logwriter is inspired by the description of the "back pressure" problem from the book [Go in Practice](http://goinpracticebook.com/).

## Usage
Create and update metrics:
```
	var skipCount int
	var errorCount int

	fSkipCounter := func(n int) { skipCount += n }
	fErrorCounter := func(out io.Writer) { errorCount += 1 }

	file1, _ := os.OpenFile("/logs/application.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	// Create a new LogWriter
	logwriter := New(LogConfig{Out: file1,
		SkipHandler:       fSkipCounter,
		WriteErrorHandler: fErrorCounter})

	// write a records to the /logs/application.log
	logwriter.Write([]byte("record1"))

	// Reopen log file
	file2, _ := os.OpenFile("/logs/application.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	go logwriter.Reset(file2)

	logwriter.Write([]byte("record2"))
```
# Installation
```
go get github.com/oleg-safonov/logwriter
```

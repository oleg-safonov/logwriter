// Package logwriter provides a solution to the "back pressure" problem when writing logs.
// Should your application crash if the disk with the log file is full?
// Should the application slow down if logging to disk does not keep up with new portions of logs?
// Package logwriter provides a LogWriter type with a circular buffer for logs that are written to the output io.Writer whenever possible.
// If the buffer overflows, the new record is skipped and one of SkipHandler or WriteErrorHandler is called
package logwriter

import (
	"io"
	"sync"
	"time"
)

const (
	defaultMaxBufSize      = 32 * (1 << 20) // 32 MB
	defaultMaxRecordsInBuf = 500000
	defaultFlashPeriod     = 100 * time.Millisecond
)

type part struct {
	pBuf *[]byte
	sPos int
	ePos int
	out  io.Writer
}

func (p *part) setPart(b *[]byte, s int, e int, o io.Writer) {
	p.pBuf = b
	p.sPos = s
	p.ePos = e
	p.out = o
}

// LogConfig encapsulates initializing parameters for the LogWriter.
// The most important is Out, there LogWriter tries to write logs. Out is the only required parameter.
// Callback WriteErrorHandler is called if an error occurred while writing to the Out.
// Callback SkipHandler is called if there is not enough space in the internal buffer for a new record.
// Callbacks SkipHandler or WriteErrorHandler can be used to notify about problems in logging, for example, in graphite or by email.
// In some cases, WriteErrorHandler can be used for re-opening a file or a network connection.
// Do not try to write to the log from SkipHandler or WriteErrorHandler, this can be dangerous.
// Parameters MaxBufSize and MaxRecordsInBuf allow you to control the size of the buffer.
// LogWriter tries to send large chunks to Out, but if 4096 bytes is not entered and there is no new data, the buffer will be written after FlashPeriod.
type LogConfig struct {
	Out               io.Writer
	WriteErrorHandler func(io.Writer)
	SkipHandler       func(int)
	MaxBufSize        int
	MaxRecordsInBuf   int
	FlashPeriod       time.Duration
}

// LogWriter encapsulates the circular buffer for fast writes to memory. LogWriter implements io.Writer interface.
// Multiple goroutines may invoke methods on a LogWriter simultaneously.
type LogWriter struct {
	out io.Writer
	buf *[]byte

	skipHandler       func(int)
	writeErrorHandler func(io.Writer)

	muInput      sync.Mutex
	inputRecords chan part
	ioInfo       chan struct{}

	muInternal sync.Mutex
	startPos   int
	endPos     int
	skipping   bool

	maxBufSize      int
	maxRecordsInBuf int
	flashPeriod     time.Duration
}

// New creates a new LogWriter with parameters from LogConfig.
func New(config LogConfig) *LogWriter {

	l := &LogWriter{out: config.Out,
		maxBufSize:      config.MaxBufSize,
		maxRecordsInBuf: config.MaxRecordsInBuf,
		flashPeriod:     config.FlashPeriod}

	if l.maxBufSize == 0 {
		l.maxBufSize = defaultMaxBufSize
	}

	if l.maxRecordsInBuf == 0 {
		l.maxRecordsInBuf = defaultMaxRecordsInBuf
	}

	if l.flashPeriod == 0 {
		l.flashPeriod = defaultFlashPeriod
	}

	b := make([]byte, l.maxBufSize)
	l.buf = &b
	l.skipHandler = config.SkipHandler
	l.writeErrorHandler = config.WriteErrorHandler
	l.inputRecords = make(chan part, l.maxRecordsInBuf+1)
	l.muInput = sync.Mutex{}
	l.muInternal = sync.Mutex{}
	l.ioInfo = make(chan struct{}, 2)
	go l.ioHandler(l.buf, l.out)
	return l
}

// Reset sets a new destination for LogWriter.
// Reset returns control only when all records in old Out are written.
// After returning from the Reset old Out can be closed.
func (l *LogWriter) Reset(out io.Writer) {
	l.reset(out)
	// wait to write all records to old io.Writer
	<-l.ioInfo
}

func (l *LogWriter) reset(out io.Writer) {
	l.muInternal.Lock()
	defer l.muInternal.Unlock()

	b := make([]byte, l.maxBufSize)
	l.buf = &b
	l.startPos = 0
	l.endPos = 0
	l.out = out
	l.skipping = false

	// write special null part for detect reopen log file
	var newpart part
	newpart.setPart(l.buf, 0, 0, l.out)
	l.inputRecords <- newpart
}

// Write appends the contents of p to the circular buffer.
// The return value n is the length of p; err is always nil.
func (l *LogWriter) Write(p []byte) (n int, err error) {
	lenP := len(p)
	if lenP < 1 {
		return 0, nil
	}

	l.muInput.Lock()
	defer l.muInput.Unlock()

	buffers, count := l.allocMem(lenP)

	if count == 0 {
		// always return "ok"
		if l.skipHandler != nil {
			l.skipHandler(1)
		}
		return lenP, nil
	}

	for i := 0; i < count; i++ {
		b := &buffers[i]
		copy((*b.pBuf)[b.sPos:b.ePos], p[:b.ePos-b.sPos])
		l.inputRecords <- buffers[i]
		p = p[b.ePos-b.sPos:]
	}

	return lenP, nil
}

func (l *LogWriter) allocMem(lenP int) (freeSlice [2]part, n int) {
	var freeBytes int

	l.muInternal.Lock()
	defer l.muInternal.Unlock()

	if l.skipping == true {
		return
	}

	freeBytes = l.freeSize()

	if freeBytes >= lenP && len(l.inputRecords) < l.maxRecordsInBuf {
		oldEnd := l.endPos
		l.endPos = (l.endPos + lenP) % l.maxBufSize

		if oldEnd < l.endPos {
			//freeSlice[0] = l.buf[oldEnd:l.endPos]
			freeSlice[0].setPart(l.buf, oldEnd, l.endPos, l.out)
			n = 1
		} else {
			//freeSlice[0] = l.buf[oldEnd:]
			freeSlice[0].setPart(l.buf, oldEnd, len(*l.buf), l.out)
			n = 1
			if l.endPos > 0 {
				//freeSlice[1] = l.buf[:l.endPos]
				freeSlice[1].setPart(l.buf, 0, l.endPos, l.out)
				n = 2
			}
		}
	} else {
		l.skipping = true
	}
	return
}

func (l *LogWriter) freeSize() int {
	if l.startPos <= l.endPos {
		return l.maxBufSize - (l.endPos - l.startPos) - 1
	} else {
		return l.startPos - l.endPos - 1
	}
}

func (l *LogWriter) ioHandler(cBuf *[]byte, out io.Writer) {
	var s, e int
	ticker := time.NewTicker(l.flashPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if s < e {
				l.write((*cBuf)[s:e], out)
				l.freeMem(cBuf, e-s)
				s = e
			}
		case p := <-l.inputRecords:
			if p.pBuf != cBuf {
				if s < e {
					l.write((*cBuf)[s:e], out)
				}
				l.ioInfo <- struct{}{}
				cBuf = p.pBuf
				out = p.out
				s = p.sPos
				e = p.sPos
			}

			if e != p.sPos {
				l.write((*cBuf)[s:e], out)
				l.freeMem(cBuf, e-s)
				s = p.sPos
				e = p.sPos
			}

			if p.ePos-s < 4096 {
				e = p.ePos
			} else {
				l.write((*cBuf)[s:p.ePos], out)
				l.freeMem(cBuf, p.ePos-s)
				s = p.ePos
				e = p.ePos
			}
		}
	}
}

func (l *LogWriter) freeMem(cBuf *[]byte, lenP int) {
	l.muInternal.Lock()
	defer l.muInternal.Unlock()
	if cBuf != l.buf {
		return
	}
	l.startPos = (l.startPos + lenP) % l.maxBufSize
	if l.skipping == true && l.freeSize() >= (l.maxBufSize/2) && len(l.inputRecords) < (l.maxRecordsInBuf/2) {
		l.skipping = false
	}
}

func (l *LogWriter) write(p []byte, out io.Writer) {
	defer func() {
		if p := recover(); p != nil {
			if l.writeErrorHandler != nil {
				l.writeErrorHandler(out)
			}
		}
	}()

	_, err := out.Write(p)
	if err != nil {
		if l.writeErrorHandler != nil {
			l.writeErrorHandler(out)
		}
	}
}

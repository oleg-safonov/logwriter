package logwriter

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"
	"time"
)

func testSleep(times int) {
	time.Sleep(time.Duration(times) * time.Millisecond)
}

type testBuffer struct {
	buf      bytes.Buffer
	delay    time.Duration
	failbit  bool
	panicbit bool
}

func (tb *testBuffer) Write(p []byte) (int, error) {
	if tb.failbit {
		return 0, fmt.Errorf("write error")
	}

	if tb.panicbit {
		panic("write error")
		return 0, fmt.Errorf("write error")
	}
	time.Sleep(tb.delay)
	tb.buf.Write(p)
	return len(p), nil
}

func ExampleLogWriter() {
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
}

func TestCreateWriter(t *testing.T) {
	var tb testBuffer
	lg := New(LogConfig{Out: &tb})
	lg.Write([]byte("test"))
	testSleep(200)
}

func TestZeroBuffer(t *testing.T) {
	var tb testBuffer
	tb.delay = 30 * time.Millisecond
	lg := New(LogConfig{Out: &tb, MaxBufSize: 8, MaxRecordsInBuf: 3})

	for i := 0; i < 100; i++ {
		lg.Write([]byte(""))
	}
	lg.Write([]byte("test"))
	testSleep(200)

	if tb.buf.String() != "test" {
		t.Error("Expected output = test, got", tb.buf.String())
	}
}

func TestBufferOverflow(t *testing.T) {
	var skipCount int
	var errorCount int

	fSkipCounter := func(n int) { skipCount += n }
	fErrorCounter := func(out io.Writer) { errorCount += 1 }

	var tb testBuffer
	tb.delay = 30 * time.Millisecond
	lg := New(LogConfig{Out: &tb,
		MaxBufSize:        8,
		MaxRecordsInBuf:   3,
		SkipHandler:       fSkipCounter,
		WriteErrorHandler: fErrorCounter})

	lg.Write([]byte("t1"))
	lg.Write([]byte("t2"))
	lg.Write([]byte("t3"))
	lg.Write([]byte("t4"))

	testSleep(200)
	if tb.buf.String() != "t1t2t3" {
		t.Error("Expected output = t1t2t3, got", tb.buf.String())
	}

	lg.Write([]byte("test1"))
	lg.Write([]byte("test2"))
	lg.Write([]byte("test3"))
	testSleep(200)
	lg.Write([]byte("test4"))
	testSleep(200)
	if tb.buf.String() != "t1t2t3test1test4" {
		t.Error("Expected output = t1t2t3test1test4, got", tb.buf.String())
	}

	if errorCount != 0 {
		t.Error("Expected errorCount = 0, got", errorCount)
	}

	if skipCount != 3 {
		t.Error("Expected skipCount = 3, got", skipCount)
	}
}

func TestWriteError(t *testing.T) {
	var skipCount int
	var errorCount int

	var tb testBuffer
	tb.delay = 30 * time.Millisecond

	fSkipCounter := func(n int) { skipCount += n }
	fErrorCounter := func(out io.Writer) { errorCount += 1 }

	lg := New(LogConfig{Out: &tb,
		MaxBufSize:        25,
		MaxRecordsInBuf:   5,
		SkipHandler:       fSkipCounter,
		WriteErrorHandler: fErrorCounter})

	lg.Write([]byte("test1"))
	lg.Write([]byte("test2"))
	lg.Write([]byte("test3"))
	testSleep(200)
	tb.failbit = true
	lg.Write([]byte("test4"))
	testSleep(300)
	if tb.buf.String() != "test1test2test3" {
		t.Error("Expected output = test1test2test3, got", tb.buf.String())
	}

	if errorCount != 1 {
		t.Error("Expected errorCount = 1, got", errorCount)
	}

	if skipCount != 0 {
		t.Error("Expected skipCount = 0, got", skipCount)
	}
}

func TestWritePanic(t *testing.T) {
	var skipCount int
	var errorCount int

	fSkipCounter := func(n int) { skipCount += n }
	fErrorCounter := func(out io.Writer) { errorCount += 1 }

	var tb testBuffer
	tb.delay = 30 * time.Millisecond
	lg := New(LogConfig{Out: &tb,
		MaxBufSize:        25,
		MaxRecordsInBuf:   5,
		SkipHandler:       fSkipCounter,
		WriteErrorHandler: fErrorCounter})

	lg.Write([]byte("test1"))
	lg.Write([]byte("test2"))
	lg.Write([]byte("test3"))
	testSleep(200)
	tb.panicbit = true
	lg.Write([]byte("test4"))
	testSleep(300)
	if tb.buf.String() != "test1test2test3" {
		t.Error("Expected output = test1test2test3, got", tb.buf.String())
	}

	if errorCount != 1 {
		t.Error("Expected errorCount = 1, got", errorCount)
	}

	if skipCount != 0 {
		t.Error("Expected skipCount = 0, got", skipCount)
	}
}

func TestReset(t *testing.T) {
	var tb1 testBuffer
	var tb2 testBuffer
	var tb3 testBuffer
	tb1.delay = 200 * time.Millisecond
	tb2.delay = 200 * time.Millisecond
	tb3.delay = 200 * time.Millisecond

	lg := New(LogConfig{Out: &tb1, FlashPeriod: 100 * time.Millisecond})

	lg.Write([]byte("test1"))
	testSleep(20)

	go lg.Reset(&tb2)
	testSleep(20)

	lg.Write([]byte("test2"))
	testSleep(20)

	go lg.Reset(&tb3)
	testSleep(400)

	if tb1.buf.String() != "test1" {
		t.Error("Expected output = test1, got", tb1.buf.String())
	}

	if tb2.buf.String() != "test2" {
		t.Error("Expected output = test2, got", tb2.buf.String())
	}
}

func TestReset2(t *testing.T) {
	var skipCount int
	var errorCount int

	fSkipCounter := func(n int) { skipCount += n }
	fErrorCounter := func(out io.Writer) { errorCount += 1 }

	var tb1 testBuffer
	var tb2 testBuffer
	tb1.delay = 100 * time.Millisecond
	tb2.delay = 100 * time.Millisecond
	lg := New(LogConfig{Out: &tb1,
		MaxBufSize:        25,
		MaxRecordsInBuf:   5,
		FlashPeriod:       300 * time.Millisecond,
		SkipHandler:       fSkipCounter,
		WriteErrorHandler: fErrorCounter})

	lg.Write([]byte("test1"))
	lg.Write([]byte("test2"))
	testSleep(10)
	go lg.Reset(&tb2)
	testSleep(100)
	lg.Write([]byte("test3"))
	testSleep(300)

	if tb1.buf.String() != "test1test2" {
		t.Error("Expected output = test1test2, got", tb1.buf.String())
	}

	if errorCount != 0 {
		t.Error("Expected errorCount = 0, got", errorCount)
	}

	if skipCount != 0 {
		t.Error("Expected skipCount = 0, got", skipCount)
	}
}

func Test4kDump(t *testing.T) {
	var skipCount int
	var errorCount int

	fSkipCounter := func(n int) { skipCount += n }
	fErrorCounter := func(out io.Writer) { errorCount += 1 }

	var tb testBuffer
	tb.delay = 100 * time.Millisecond
	lg := New(LogConfig{Out: &tb,
		SkipHandler:       fSkipCounter,
		WriteErrorHandler: fErrorCounter})

	for i := 0; i < 1000; i++ {
		lg.Write([]byte("test1"))
	}

	testSleep(1200)

	if len(tb.buf.String()) != 5000 {
		t.Error("Expected output length = 5000, got", len(tb.buf.String()))
	}

	if errorCount != 0 {
		t.Error("Expected errorCount = 0, got", errorCount)
	}

	if skipCount != 0 {
		t.Error("Expected skipCount = 0, got", skipCount)
	}
}

func benchmarkWrite(b *testing.B, line []byte) {
	var skipCount int
	var errorCount int

	fSkipCounter := func(n int) { skipCount += n }
	fErrorCounter := func(out io.Writer) { errorCount += 1 }

	var tb testBuffer
	lg := New(LogConfig{Out: &tb,
		MaxBufSize:        100 * b.N,
		MaxRecordsInBuf:   5000000,
		SkipHandler:       fSkipCounter,
		WriteErrorHandler: fErrorCounter})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lg.Write(line)
	}
}

func BenchmarkWrite4b(b *testing.B) {
	benchmarkWrite(b, []byte("test"))
}

func BenchmarkWrite100b(b *testing.B) {
	line := make([]byte, 100)
	for i := 0; i < 100; i++ {
		line[i] = 't'
	}
	benchmarkWrite(b, line)
}

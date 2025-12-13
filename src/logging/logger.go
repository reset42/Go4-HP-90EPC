package logging

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"hp90epc/model"
)

type LogStatus struct {
	Active     bool   `json:"active"`
	File       string `json:"file"`
	IntervalMs int    `json:"interval_ms"`
}

type Logger struct {
	active bool

	dir       string
	interval  time.Duration
	lastWrite time.Time

	file        *os.File
	csv         *csv.Writer
	currentName string
}

func NewLogger(dir string, interval time.Duration) *Logger {
	return &Logger{
		dir:      dir,
		interval: interval,
	}
}

func (l *Logger) Start() error {
	if l.active {
		return nil
	}

	if err := os.MkdirAll(l.dir, 0o755); err != nil {
		return fmt.Errorf("mkdir logs: %w", err)
	}

	ts := time.Now().Format("2006-01-02_15-04-05")
	name := fmt.Sprintf("hp90epc_%s.csv", ts)
	full := filepath.Join(l.dir, name)

	f, err := os.Create(full)
	if err != nil {
		return fmt.Errorf("create log file: %w", err)
	}

	w := csv.NewWriter(f)
	header := []string{
		"value", "value_str", "unit", "mode",
		"auto", "hold", "rel", "low_batt",
		"raw",
	}
	if err := w.Write(header); err != nil {
		_ = f.Close()
		return fmt.Errorf("write header: %w", err)
	}
	w.Flush()

	l.file = f
	l.csv = w
	l.currentName = name
	l.lastWrite = time.Time{}
	l.active = true
	return nil
}

func (l *Logger) Stop() error {
	if !l.active {
		return nil
	}
	l.active = false

	if l.csv != nil {
		l.csv.Flush()
	}
	if l.file != nil {
		if err := l.file.Close(); err != nil {
			return err
		}
	}

	l.csv = nil
	l.file = nil
	return nil
}

func (l *Logger) Status() LogStatus {
	return LogStatus{
		Active:     l.active,
		File:       l.currentName,
		IntervalMs: int(l.interval / time.Millisecond),
	}
}

func (l *Logger) SetInterval(ms int) {
	if ms <= 0 {
		ms = 1000
	}
	l.interval = time.Duration(ms) * time.Millisecond
}

func (l *Logger) Push(m *model.Measurement) {
	if m == nil || !l.active || l.csv == nil {
		return
	}

	if l.interval > 0 && !l.lastWrite.IsZero() {
		if time.Since(l.lastWrite) < l.interval {
			return
		}
	}

	valStr := ""
	if m.Value != nil {
		valStr = fmt.Sprintf("%g", *m.Value)
	}

	record := []string{
		valStr,
		m.ValueStr,
		m.Unit,
		m.Mode,
		boolToStr(m.Auto),
		boolToStr(m.Hold),
		boolToStr(m.Rel),
		boolToStr(m.LowBatt),
		m.RawHex,
	}

	if err := l.csv.Write(record); err != nil {
		fmt.Fprintf(os.Stderr, "logger write error: %v\n", err)
		l.active = false
		return
	}
	l.csv.Flush()
	l.lastWrite = time.Now()
}

func boolToStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func (l *Logger) ListFiles() ([]string, error) {
	if err := os.MkdirAll(l.dir, 0o755); err != nil {
		return nil, err
	}
	ents, err := os.ReadDir(l.dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		out = append(out, e.Name())
	}
	return out, nil
}

func (l *Logger) ReadFile(name string) ([]byte, error) {
	full := filepath.Join(l.dir, name)
	return os.ReadFile(full)
}

func (l *Logger) Tail(name string, maxLines int) ([]string, error) {
	full := filepath.Join(l.dir, name)
	f, err := os.Open(full)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if maxLines <= 0 {
		maxLines = 200
	}

	sc := bufio.NewScanner(f)
	buf := make([]string, 0, maxLines)

	for sc.Scan() {
		line := sc.Text()
		if len(buf) < maxLines {
			buf = append(buf, line)
		} else {
			copy(buf, buf[1:])
			buf[maxLines-1] = line
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return buf, nil
}


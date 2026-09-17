package output

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/faceair/clash-speedtest/core/speedtester"
)

// TSVWriter writes tab-separated values output without ANSI colors
// It outputs the header immediately on creation, then streams rows as they complete
type TSVWriter struct {
	output        io.Writer
	mode          speedtester.SpeedMode
	metrics       speedtester.MetricSet
	headerWritten bool
}

// NewTSVWriter creates a new TSV writer and writes the header immediately
func NewTSVWriter(output io.Writer, mode speedtester.SpeedMode) (*TSVWriter, error) {
	return NewTSVWriterWithMetrics(output, mode, speedtester.MetricSet{})
}

func NewTSVWriterWithMetrics(output io.Writer, mode speedtester.SpeedMode, metrics speedtester.MetricSet) (*TSVWriter, error) {
	w := &TSVWriter{
		output:  output,
		mode:    mode,
		metrics: metrics,
	}
	if err := w.writeHeader(); err != nil {
		return nil, fmt.Errorf("failed to write TSV header: %w", err)
	}
	return w, nil
}

// writeHeader writes the TSV header row
func (w *TSVWriter) writeHeader() error {
	if w.headerWritten {
		return nil
	}
	headers := GetHeadersWithMetrics(w.mode, w.metrics)
	_, err := w.output.Write([]byte(strings.Join(headers, "\t") + "\n"))
	if err != nil {
		return fmt.Errorf("write header failed: %w", err)
	}
	w.headerWritten = true
	return nil
}

// WriteRow writes a single result row to TSV output
// The index parameter is used for the sequence number column
func (w *TSVWriter) WriteRow(result *speedtester.Result, index int) error {
	if result == nil {
		return errors.New("cannot write nil result")
	}
	row := FormatRowWithMetrics(result, w.mode, index, w.metrics)
	_, err := w.output.Write([]byte(strings.Join(row, "\t") + "\n"))
	if err != nil {
		return fmt.Errorf("write row for proxy %q (index %d) failed: %w", result.ProxyName, index, err)
	}
	return nil
}

// WriteRows writes multiple result rows to TSV output
func (w *TSVWriter) WriteRows(results []*speedtester.Result) error {
	for i, result := range results {
		if err := w.WriteRow(result, i); err != nil {
			return err
		}
	}
	return nil
}

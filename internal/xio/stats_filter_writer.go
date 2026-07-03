package xio

import (
	"bytes"
	"strconv"
	"strings"
)

const statsPrefix = "__RAGETA_STATS__ "

type StatsFilterWriter struct {
	inner   writer
	onStats func(cpu, mem, netRx, netTx int64) error
	buf     []byte
}

type writer interface {
	Write([]byte) (int, error)
}

func NewStatsFilterWriter(inner writer, onStats func(cpu, mem, netRx, netTx int64) error) *StatsFilterWriter {
	return &StatsFilterWriter{inner: inner, onStats: onStats}
}

func (w *StatsFilterWriter) Write(b []byte) (int, error) {
	w.buf = append(w.buf, b...)
	for {
		idx := bytes.IndexByte(w.buf, '\n')
		if idx < 0 {
			break
		}
		line := w.buf[:idx+1]
		w.buf = w.buf[idx+1:]

		if strings.HasPrefix(string(line), statsPrefix) {
			w.parseStats(strings.TrimSpace(string(line)[len(statsPrefix):]))
		} else {
			if _, err := w.inner.Write(line); err != nil {
				return 0, err
			}
		}
	}
	return len(b), nil
}

func (w *StatsFilterWriter) parseStats(s string) {
	var cpu, mem, netRx, netTx int64
	for _, field := range strings.Fields(s) {
		k, v, ok := strings.Cut(field, "=")
		if !ok {
			continue
		}
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			continue
		}
		switch k {
		case "cpu":
			cpu = n
		case "mem":
			mem = n
		case "net_rx":
			netRx = n
		case "net_tx":
			netTx = n
		}
	}
	w.onStats(cpu, mem, netRx, netTx)
}

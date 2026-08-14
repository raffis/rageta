// Package stats samples per-task resource usage (CPU, memory, network) from
// cgroup and /proc/net/dev files. It replaces what used to be a shell script
// baked into the task's container and polled via stderr markers: the same
// sampling logic now runs natively in Go.
package stats

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	cgroupV2CPUStatPath  = "/sys/fs/cgroup/cpu.stat"
	cgroupV2MemStatPath  = "/sys/fs/cgroup/memory.current"
	cgroupV1CPUUsagePath = "/sys/fs/cgroup/cpuacct/cpuacct.usage"
	cgroupV1MemUsagePath = "/sys/fs/cgroup/memory/memory.usage_in_bytes"
	procNetDevPath       = "/proc/net/dev"
	loopbackPrefix       = "lo:"
)

// NewCollector returns a Collector ready to take its first sample. The first
// call to Current always reports zero CPU millicores and zero net bytes,
// since both are derived from the delta against a previous sample/baseline.
func NewCollector() *Collector {
	return &Collector{}
}

// Collector samples cgroup CPU/memory usage and network traffic, rebasing
// CPU and network counters against the previous sample so that Current
// reports deltas rather than cumulative absolutes.
type Collector struct {
	initialized  bool
	prevCPUUsec  uint64
	prevSampleAt time.Time
	netRxBase    int64
	netTxBase    int64
}

// Current takes a new sample and returns the resulting Sample. CPU is
// reported as Kubernetes-style millicores (1000m == one full core-second
// consumed per second of wall-clock time since the previous sample).
func (c *Collector) Current() (*Sample, error) {
	usec, mem, err := readCPUAndMem()
	if err != nil {
		return nil, err
	}

	netRx, netTx, err := readNetBytes()
	if err != nil {
		return nil, err
	}

	now := time.Now()

	dp := &Sample{
		MemBytes: mem,
	}

	if !c.initialized {
		c.netRxBase = netRx
		c.netTxBase = netTx
		c.initialized = true
	} else {
		elapsed := now.Sub(c.prevSampleAt).Seconds()
		if elapsed > 0 && usec >= c.prevCPUUsec {
			deltaUsec := usec - c.prevCPUUsec
			dp.CPUMillicores = int64(float64(deltaUsec) / 1000 / elapsed)
		}
	}

	dp.NetRxBytes = max(netRx-c.netRxBase, 0)
	dp.NetTxBytes = max(netTx-c.netTxBase, 0)

	c.prevCPUUsec = usec
	c.prevSampleAt = now

	return dp, nil
}

// readCPUAndMem returns cumulative CPU usage in microseconds and current
// memory usage in bytes, preferring cgroup v2 and falling back to v1. It
// returns zero values, not an error, when neither cgroup interface is
// present, matching the shell reporter's original behavior of reporting
// zeroes rather than failing.
func readCPUAndMem() (usec uint64, mem int64, err error) {
	if data, err := os.ReadFile(cgroupV2CPUStatPath); err == nil {
		usec = parseCPUStatUsec(data)
		mem = readInt64File(cgroupV2MemStatPath)
		return usec, mem, nil
	}

	if data, err := os.ReadFile(cgroupV1CPUUsagePath); err == nil {
		ns, parseErr := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
		if parseErr != nil {
			return 0, 0, fmt.Errorf("parse %s: %w", cgroupV1CPUUsagePath, parseErr)
		}
		usec = ns / 1000
		mem = readInt64File(cgroupV1MemUsagePath)
		return usec, mem, nil
	}

	return 0, 0, nil
}

// parseCPUStatUsec extracts the usage_usec field from a cgroup v2 cpu.stat
// file. Missing or malformed content yields 0, mirroring the original awk
// one-liner which silently produced an empty value in that case.
func parseCPUStatUsec(data []byte) uint64 {
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), " ")
		if !ok || k != "usage_usec" {
			continue
		}
		n, err := strconv.ParseUint(strings.TrimSpace(v), 10, 64)
		if err != nil {
			return 0
		}
		return n
	}
	return 0
}

// readInt64File reads a file containing a single integer, returning 0 if the
// file is missing or unparseable.
func readInt64File(path string) int64 {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	n, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// readNetBytes sums received/transmitted bytes across all non-loopback
// interfaces listed in /proc/net/dev.
func readNetBytes() (rx, tx int64, err error) {
	f, err := os.Open(procNetDevPath)
	if err != nil {
		return 0, 0, nil
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		iface, fields, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		iface = strings.TrimSpace(iface)
		if iface == "" || strings.HasPrefix(iface+":", loopbackPrefix) {
			continue
		}

		cols := strings.Fields(fields)
		// /proc/net/dev columns: rx bytes is field 0, tx bytes is field 8.
		if len(cols) < 9 {
			continue
		}

		rxN, err := strconv.ParseInt(cols[0], 10, 64)
		if err != nil {
			continue
		}
		txN, err := strconv.ParseInt(cols[8], 10, 64)
		if err != nil {
			continue
		}

		rx += rxN
		tx += txN
	}

	if err := sc.Err(); err != nil {
		return 0, 0, fmt.Errorf("read %s: %w", procNetDevPath, err)
	}

	return rx, tx, nil
}

// Run  continuously collects and writes samples to w.
func Run(ctx context.Context, w io.Writer, interval time.Duration) error {
	c := NewCollector()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			dp, err := c.Current()
			if err != nil {
				return err
			}

			b, err := dp.Marshal()
			if err != nil {
				return err
			}

			w.Write(b)
		}
	}
}

// Sample is a single resource-usage sample.
type Sample struct {
	CPUMillicores int64
	MemBytes      int64
	NetRxBytes    int64
	NetTxBytes    int64
}

// Unmarshal decodes b, as produced by Marshal, into c.
func (c *Sample) Unmarshal(b []byte) error {
	if len(b) != 32 {
		return fmt.Errorf("expected %d bytes, got %d", 32, len(b))
	}

	c.CPUMillicores = int64(binary.BigEndian.Uint64(b[0:8]))
	c.MemBytes = int64(binary.BigEndian.Uint64(b[8:16]))
	c.NetRxBytes = int64(binary.BigEndian.Uint64(b[16:24]))
	c.NetTxBytes = int64(binary.BigEndian.Uint64(b[24:32]))

	return nil
}

// Marshal encodes c as four fixed-width big-endian uint64 fields, in the
// same order as Sample' fields.
func (c *Sample) Marshal() ([]byte, error) {
	b := make([]byte, 32)

	binary.BigEndian.PutUint64(b[0:8], uint64(c.CPUMillicores))
	binary.BigEndian.PutUint64(b[8:16], uint64(c.MemBytes))
	binary.BigEndian.PutUint64(b[16:24], uint64(c.NetRxBytes))
	binary.BigEndian.PutUint64(b[24:32], uint64(c.NetTxBytes))

	return b, nil
}

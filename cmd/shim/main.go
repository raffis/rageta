// Command shim runs a script as a child process while sampling container
// resource stats alongside it and detecting its non-loopback IP. The
// script's stdout passes straight through to shim's own stdout; the
// script's stderr, the periodic stats samples, and the detected IP are all
// multiplexed onto shim's own stderr via xio.Mux, so a single fd carries all
// three without ever splicing one mid-line into another.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/raffis/rageta/internal/stats"
	"github.com/raffis/rageta/internal/xio"
)

const (
	ipPollInterval  = 100 * time.Millisecond
	ipDetectTimeout = 5 * time.Second
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: shim [args] <script> [script args...]")
		return 2
	}

	fDetectIP := flag.Bool("ip", false, "Detect non loopback IP")
	fStats := flag.Bool("stats", false, "Write stats to stderr")
	fExitCodePath := flag.String("exitcodefile", "", "Return 0 and write real code to the given path")
	fStatsInterval := flag.Duration("stats-interval", time.Second*1, "Stats interval")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	tail := flag.Args()
	cmd := exec.Command(tail[0], tail[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Env = os.Environ()

	stderrR, stderrW := io.Pipe()
	cmd.Stderr = stderrW

	muxers := []xio.MuxSource{
		{ID: xio.StreamOutput, R: stderrR},
	}

	if fStats != nil && *fStats {
		statsCtx, cancelStats := context.WithCancel(context.Background())
		defer cancelStats()

		statsR, statsW := io.Pipe()
		statsDone := make(chan struct{})
		go func() {
			defer close(statsDone)
			if err := stats.Run(statsCtx, statsW, *fStatsInterval); err != nil && !errors.Is(err, context.Canceled) {
				_ = statsW.CloseWithError(err)
				return
			}
			_ = statsW.Close()
		}()

		muxers = append(muxers, xio.MuxSource{ID: xio.StreamStats, R: statsR})
	}

	if fDetectIP != nil && *fDetectIP {
		ipCtx, cancelIP := context.WithCancel(context.Background())
		defer cancelIP()

		ipR, ipW := io.Pipe()
		ipDone := make(chan struct{})
		go func() {
			defer close(ipDone)
			if ip, err := detectIP(ipCtx); err == nil {
				_, _ = ipW.Write([]byte(ip))
			}
			_ = ipW.Close()
		}()

		muxers = append(muxers, xio.MuxSource{ID: xio.StreamIP, R: ipR})
	}

	muxDone := make(chan error, 1)
	go func() {
		muxDone <- xio.Mux(os.Stderr, muxers...)
	}()

	cleanup := func() {
		_ = stderrW.Close()
		/*cancelStats()
		cancelIP()
		<-statsDone
		<-ipDone
		<-muxDone*/
	}

	if err := cmd.Start(); err != nil {
		cleanup()
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	go func() {
		<-ctx.Done()
		_ = cmd.Process.Signal(syscall.SIGTERM)
	}()

	waitErr := cmd.Wait()
	cleanup()

	f, _ := os.OpenFile(*fExitCodePath, os.O_CREATE|os.O_WRONLY, 0644)
	defer f.Close()

	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		if fExitCodePath == nil || *fExitCodePath == "" {
			return exitErr.ExitCode()
		}

		fmt.Fprintf(f, "%d", exitErr.ExitCode())
		return 0
	}

	if waitErr != nil {
		fmt.Fprintf(f, "%d", 1)
		fmt.Fprintln(os.Stderr, waitErr)
		return 1
	}

	fmt.Fprintf(f, "%d", 0)
	return 0
}

// detectIP polls the local network interfaces until it finds a non-loopback
// IPv4 address, or ctx is done.
func detectIP(ctx context.Context) (string, error) {
	ticker := time.NewTicker(ipPollInterval)
	defer ticker.Stop()

	deadline := time.After(ipDetectTimeout)

	for {
		if ip := firstNonLoopbackIPv4(); ip != "" {
			return ip, nil
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-deadline:
			return "", fmt.Errorf("timed out detecting a non-loopback IP")
		case <-ticker.C:
		}
	}
}

func firstNonLoopbackIPv4() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch a := addr.(type) {
			case *net.IPNet:
				ip = a.IP
			case *net.IPAddr:
				ip = a.IP
			}

			if ip4 := ip.To4(); ip4 != nil {
				return ip4.String()
			}
		}
	}

	return ""
}

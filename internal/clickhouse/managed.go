package clickhouse

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/SEObserver/crawlobserver/internal/applog"
)

// ManagedServer manages a ClickHouse subprocess.
type ManagedServer struct {
	cmd        *exec.Cmd
	tcpPort    int
	httpPort   int
	dataDir    string
	binaryPath string

	// Watchdog
	watchdogStop chan struct{}
	watchdogMu   sync.Mutex
	stopping     bool
}

// NewManagedServer creates a new managed ClickHouse server configuration.
func NewManagedServer(dataDir string) *ManagedServer {
	return &ManagedServer{dataDir: dataDir}
}

// Start launches the ClickHouse server process, waits for readiness, and
// starts the auto-restart watchdog.
func (m *ManagedServer) Start(ctx context.Context, binaryPath string) error {
	if err := m.startProcess(ctx, binaryPath); err != nil {
		return err
	}
	m.startWatchdog()
	return nil
}

// startProcess starts the ClickHouse subprocess and waits for it to be ready.
// Pure process lifecycle — no watchdog interaction. Used both by Start (public
// API, which adds the watchdog) and by the watchdog itself (which restarts the
// process without touching its own goroutine).
func (m *ManagedServer) startProcess(ctx context.Context, binaryPath string) error {
	tcpPort, httpPort, err := findAvailablePorts()
	if err != nil {
		return fmt.Errorf("finding available ports: %w", err)
	}
	m.tcpPort = tcpPort
	m.httpPort = httpPort

	configPath, err := writeConfigXML(m.dataDir, tcpPort, httpPort)
	if err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	m.binaryPath = binaryPath

	applog.Infof("clickhouse", "Starting managed ClickHouse (tcp=%d, http=%d, data=%s)", tcpPort, httpPort, m.dataDir)

	// Do NOT use exec.CommandContext: the ctx is only for the startup timeout,
	// not for the lifetime of the ClickHouse process. CommandContext kills the
	// process when the context is cancelled, which would stop ClickHouse as soon
	// as setupClickHouse returns and its deferred cancel() fires.
	m.cmd = exec.Command(binaryPath, "server", "--config-file="+configPath)
	m.cmd.Stdout = os.Stdout
	m.cmd.Stderr = os.Stderr

	if err := m.cmd.Start(); err != nil {
		return fmt.Errorf("starting ClickHouse: %w", err)
	}

	if err := m.waitReady(30 * time.Second); err != nil {
		m.stopProcess()
		return fmt.Errorf("ClickHouse failed to start: %w", err)
	}

	applog.Infof("clickhouse", "Managed ClickHouse ready on port %d", tcpPort)
	return nil
}

// Stop gracefully stops the ClickHouse process AND the watchdog. Public API.
func (m *ManagedServer) Stop() {
	m.watchdogMu.Lock()
	m.stopping = true
	if m.watchdogStop != nil {
		close(m.watchdogStop)
		m.watchdogStop = nil
	}
	m.watchdogMu.Unlock()
	m.stopProcess()
}

// stopProcess kills the ClickHouse subprocess without touching the watchdog
// state. Used by the watchdog when it needs to restart the process.
func (m *ManagedServer) stopProcess() {
	if m.cmd == nil || m.cmd.Process == nil {
		return
	}

	applog.Info("clickhouse", "Stopping managed ClickHouse...")

	if runtime.GOOS == "windows" {
		m.cmd.Process.Kill()
		m.cmd.Wait()
		m.cmd = nil
		return
	}

	m.cmd.Process.Signal(syscall.SIGTERM)

	done := make(chan error, 1)
	go func() { done <- m.cmd.Wait() }()

	select {
	case <-done:
		applog.Info("clickhouse", "Managed ClickHouse stopped.")
	case <-time.After(10 * time.Second):
		applog.Warn("clickhouse", "ClickHouse did not stop gracefully, killing...")
		m.cmd.Process.Kill()
		<-done
	}
	m.cmd = nil
}

// TCPPort returns the native TCP port.
func (m *ManagedServer) TCPPort() int {
	return m.tcpPort
}

// HTTPPort returns the HTTP port.
func (m *ManagedServer) HTTPPort() int {
	return m.httpPort
}

// Restart stops and restarts the managed ClickHouse server. Public API: also
// resets the watchdog so it keeps supervising the new process.
func (m *ManagedServer) Restart(ctx context.Context) error {
	m.Stop()
	return m.Start(ctx, m.binaryPath)
}

// DataDir returns the data directory path.
func (m *ManagedServer) DataDir() string {
	return m.dataDir
}

// startWatchdog launches a background goroutine that periodically pings the
// ClickHouse TCP port and auto-restarts the process if it crashed.
//
// Why: ClickHouse 25.x on macOS occasionally crashes mid-session on JIT
// compilation errors (CANNOT_COMPILE_CODE) or background merge exceptions,
// leaving the SeeseoCrawler app in a broken state ("ClickHouse is not
// reachable") until manual app restart. The watchdog brings CH back up
// automatically so the user can keep working.
//
// Strategy: ping every 5s. On 2 consecutive failures, attempt restart.
// Backoff between restart attempts: 2s → 5s → 10s → 30s, then 30s steady.
func (m *ManagedServer) startWatchdog() {
	m.watchdogMu.Lock()
	if m.watchdogStop != nil {
		close(m.watchdogStop)
	}
	stop := make(chan struct{})
	m.watchdogStop = stop
	m.stopping = false
	m.watchdogMu.Unlock()

	go func() {
		const pingInterval = 5 * time.Second
		const failuresBeforeRestart = 2
		backoffs := []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second, 30 * time.Second}
		consecutiveFailures := 0
		restartAttempts := 0

		for {
			select {
			case <-stop:
				return
			case <-time.After(pingInterval):
			}

			m.watchdogMu.Lock()
			if m.stopping {
				m.watchdogMu.Unlock()
				return
			}
			port := m.tcpPort
			m.watchdogMu.Unlock()

			addr := fmt.Sprintf("127.0.0.1:%d", port)
			conn, err := net.DialTimeout("tcp", addr, 1500*time.Millisecond)
			if err == nil {
				conn.Close()
				consecutiveFailures = 0
				restartAttempts = 0
				continue
			}

			consecutiveFailures++
			if consecutiveFailures < failuresBeforeRestart {
				continue
			}

			applog.Warnf("clickhouse", "Watchdog detected ClickHouse down (port %d unreachable), restarting...", port)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			// Restart in place without disturbing this watchdog goroutine.
			m.stopProcess()
			err = m.startProcess(ctx, m.binaryPath)
			cancel()

			if err == nil {
				applog.Info("clickhouse", "Watchdog: ClickHouse restarted successfully.")
				consecutiveFailures = 0
				restartAttempts = 0
				continue
			}

			applog.Warnf("clickhouse", "Watchdog: restart failed (attempt %d): %v", restartAttempts+1, err)
			backoff := backoffs[len(backoffs)-1]
			if restartAttempts < len(backoffs) {
				backoff = backoffs[restartAttempts]
			}
			restartAttempts++

			select {
			case <-stop:
				return
			case <-time.After(backoff):
			}
		}
	}()
}

// waitReady polls the TCP port until ClickHouse accepts connections.
func (m *ManagedServer) waitReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	addr := fmt.Sprintf("127.0.0.1:%d", m.tcpPort)

	for time.Now().Before(deadline) {
		// Check if process died
		if m.cmd.ProcessState != nil {
			return fmt.Errorf("process exited with: %v", m.cmd.ProcessState)
		}

		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for ClickHouse on %s", addr)
}

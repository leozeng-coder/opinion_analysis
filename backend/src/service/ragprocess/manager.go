package ragprocess

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"opinion-analysis/config"
)

// RestartResult 重启 RAG 子进程结果。
type RestartResult struct {
	OK          bool   `json:"ok"`
	PID         int    `json:"pid,omitempty"`
	HealthReady bool   `json:"healthReady"`
	Starting    bool   `json:"starting,omitempty"`
	ElapsedMs   int64  `json:"elapsedMs"`
	Message     string `json:"message"`
}

// Manager 由 Go 后端托管的本机 RAG Python 子进程。
type Manager struct {
	mu  sync.Mutex
	cmd *exec.Cmd
	pid int
}

func NewManager() *Manager {
	return &Manager{}
}

func (m *Manager) ManagedEnabled() bool {
	return config.Cfg != nil && config.Cfg.RAG.Managed
}

func (m *Manager) PID() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.pid
}

func (m *Manager) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.pid > 0 && m.cmd != nil && m.cmd.Process != nil
}

// EnsureStarted 若 health 不可达则启动子进程（不杀已有外部进程）。
func (m *Manager) EnsureStarted(ctx context.Context) error {
	if !m.ManagedEnabled() {
		return nil
	}
	if healthOK(ctx, serviceURL()) {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pid > 0 {
		return nil
	}
	return m.startLocked()
}

// Restart 杀占用端口的旧进程并拉起新 RAG，轮询 /health 直到就绪。
func (m *Manager) Restart(ctx context.Context) (RestartResult, error) {
	if !m.ManagedEnabled() {
		return RestartResult{}, fmt.Errorf("RAG 进程托管未启用（config rag.managed）")
	}
	start := time.Now()
	m.mu.Lock()
	if err := m.stopLocked(); err != nil {
		m.mu.Unlock()
		return RestartResult{}, err
	}
	port, err := servicePort()
	if err != nil {
		m.mu.Unlock()
		return RestartResult{}, err
	}
	if err := killProcessesOnPort(port); err != nil {
		log.Printf("[rag-process] kill port %d: %v", port, err)
	}
	time.Sleep(400 * time.Millisecond)
	if err := m.startLocked(); err != nil {
		m.mu.Unlock()
		return RestartResult{}, err
	}
	pid := m.pid
	m.mu.Unlock()

	ready, _ := waitHealth(ctx, serviceURL(), 20*time.Second)
	elapsed := time.Since(start).Milliseconds()
	res := RestartResult{
		OK:          true,
		PID:         pid,
		HealthReady: ready,
		ElapsedMs:   elapsed,
	}
	if ready {
		res.Message = fmt.Sprintf("RAG 服务已就绪（PID %d）", pid)
		return res, nil
	}
	if m.IsRunning() || portListening(servicePortQuick()) {
		res.Starting = true
		res.Message = "RAG 进程已启动，嵌入模型加载中，请稍候刷新状态"
		return res, nil
	}
	res.OK = false
	res.Message = "RAG 进程启动失败，请查看 rag/logs/rag_service_managed.log"
	return res, fmt.Errorf("%s", res.Message)
}

func servicePortQuick() int {
	p, err := servicePort()
	if err != nil {
		return 0
	}
	return p
}

func portListening(port int) bool {
	if port <= 0 {
		return false
	}
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	conn, err := net.DialTimeout("tcp", addr, 800*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// Stop 停止托管的子进程。
func (m *Manager) Stop(ctx context.Context) error {
	if !m.ManagedEnabled() {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopLocked()
}

func (m *Manager) stopLocked() error {
	if m.cmd != nil && m.cmd.Process != nil {
		_ = m.cmd.Process.Kill()
		_, _ = m.cmd.Process.Wait()
	}
	m.cmd = nil
	m.pid = 0
	return nil
}

func (m *Manager) startLocked() error {
	root, python, script, workDir, err := resolveExec()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o755); err != nil {
		return err
	}
	logPath := filepath.Join(root, "logs", "rag_service_managed.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open log %s: %w", logPath, err)
	}

	cmd := exec.Command(python, script)
	cmd.Dir = workDir
	cmd.Env = os.Environ()
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if runtime.GOOS == "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
		}
	}

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return fmt.Errorf("start RAG: %w", err)
	}

	go func(c *exec.Cmd, lf io.Closer) {
		_ = c.Wait()
		_ = lf.Close()
		m.mu.Lock()
		if m.cmd == c {
			m.cmd = nil
			m.pid = 0
		}
		m.mu.Unlock()
	}(cmd, logFile)

	m.cmd = cmd
	m.pid = cmd.Process.Pid
	log.Printf("[rag-process] started pid=%d workdir=%s python=%s", m.pid, workDir, python)
	return nil
}

func resolveExec() (root, python, script, workDir string, err error) {
	if config.Cfg == nil {
		return "", "", "", "", fmt.Errorf("config not loaded")
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", "", "", "", err
	}
	rootRel := config.Cfg.RAG.Root
	if rootRel == "" {
		rootRel = "../rag"
	}
	root = filepath.Clean(filepath.Join(wd, rootRel))
	if st, e := os.Stat(root); e != nil || !st.IsDir() {
		return "", "", "", "", fmt.Errorf("RAG root not found: %s", root)
	}

	scriptRel := strings.TrimSpace(config.Cfg.RAG.ServerScript)
	if scriptRel == "" {
		scriptRel = "server.py"
	}
	script = filepath.Clean(filepath.Join(root, filepath.FromSlash(scriptRel)))
	if _, e := os.Stat(script); e != nil {
		return "", "", "", "", fmt.Errorf("RAG server script not found: %s", script)
	}
	workDir = filepath.Dir(script)

	if config.Cfg.RAG.Python != "" {
		python = config.Cfg.RAG.Python
	} else if runtime.GOOS == "windows" {
		python = filepath.Join(root, ".venv", "Scripts", "python.exe")
	} else {
		python = filepath.Join(root, ".venv", "bin", "python3")
		if _, e := os.Stat(python); e != nil {
			python = filepath.Join(root, ".venv", "bin", "python")
		}
	}
	if _, e := os.Stat(python); e != nil {
		return "", "", "", "", fmt.Errorf("Python not found at %s (create %s/.venv first)", python, rootRel)
	}
	return root, python, script, workDir, nil
}

func serviceURL() string {
	if config.Cfg == nil {
		return ""
	}
	return strings.TrimSpace(config.Cfg.RAG.EmbeddingServiceURL)
}

func servicePort() (int, error) {
	raw := serviceURL()
	if raw == "" {
		return 0, fmt.Errorf("rag.embedding_service_url not configured")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return 0, err
	}
	p := u.Port()
	if p == "" {
		if u.Scheme == "https" {
			p = "443"
		} else {
			p = "80"
		}
	}
	return strconv.Atoi(p)
}

func healthOK(ctx context.Context, base string) bool {
	if base == "" {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/health", nil)
	if err != nil {
		return false
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode/100 == 2
}

func waitHealth(ctx context.Context, base string, timeout time.Duration) (bool, error) {
	deadline := time.Now().Add(timeout)
	for {
		if healthOK(ctx, base) {
			return true, nil
		}
		if time.Now().After(deadline) {
			return false, fmt.Errorf("等待 RAG health 超时（%s）", timeout)
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func killProcessesOnPort(port int) error {
	if runtime.GOOS == "windows" {
		return killPortWindows(port)
	}
	return killPortUnix(port)
}

func killPortWindows(port int) error {
	out, err := exec.Command("netstat", "-ano").Output()
	if err != nil {
		return err
	}
	needle := fmt.Sprintf(":%d", port)
	seen := map[int]struct{}{}
	for _, line := range strings.Split(string(out), "\n") {
		upper := strings.ToUpper(line)
		if !strings.Contains(line, needle) || !strings.Contains(upper, "LISTENING") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		pidStr := fields[len(fields)-1]
		pid, err := strconv.Atoi(pidStr)
		if err != nil || pid <= 0 {
			continue
		}
		if _, ok := seen[pid]; ok {
			continue
		}
		seen[pid] = struct{}{}
		_ = exec.Command("taskkill", "/F", "/PID", strconv.Itoa(pid)).Run()
		log.Printf("[rag-process] taskkill pid=%d port=%d", pid, port)
	}
	return nil
}

func killPortUnix(port int) error {
	out, err := exec.Command("lsof", "-ti", fmt.Sprintf(":%d", port)).Output()
	if err != nil {
		return nil
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		pid, err := strconv.Atoi(strings.TrimSpace(line))
		if err != nil || pid <= 0 {
			continue
		}
		proc, err := os.FindProcess(pid)
		if err != nil {
			continue
		}
		_ = proc.Kill()
		log.Printf("[rag-process] killed pid=%d port=%d", pid, port)
	}
	return nil
}

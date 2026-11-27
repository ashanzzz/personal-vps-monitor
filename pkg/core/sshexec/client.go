package sshexec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/ashanzzz/personal-vps-monitor/pkg/core/audit"
	"github.com/ashanzzz/personal-vps-monitor/pkg/core/store"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers"
	"golang.org/x/crypto/ssh"
)

// ExecResult 统一 SSH 命令执行响应结果
type ExecResult struct {
	ExitCode   int    `json:"exit_code"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	DurationMs int64  `json:"duration_ms"`
}

type cachedSSHClient struct {
	client    *ssh.Client
	hostKey   string
	lastUsed  time.Time
	mu        sync.Mutex
}

var (
	poolMu            sync.Mutex
	sshPool           = make(map[string]*cachedSSHClient)
	cleanOnce         sync.Once
	lazyCatExecutorFn = defaultLazyCatExecutor
)

func defaultLazyCatExecutor(ctx context.Context, instanceID, cmd string, timeout time.Duration) (*providers.TerminalExecResult, bool, error) {
	p, ok := providers.Get("lazycat")
	if !ok {
		return nil, false, nil
	}
	te, isTE := p.(providers.TerminalExecutor)
	if !isTE {
		return nil, false, nil
	}
	res, err := te.ExecInstanceCommand(ctx, instanceID, cmd, timeout)
	return res, true, err
}

func initCleaner() {
	cleanOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				cleanIdleConnections()
			}
		}()
	})
}

func cleanIdleConnections() {
	poolMu.Lock()
	defer poolMu.Unlock()

	now := time.Now()
	for k, c := range sshPool {
		c.mu.Lock()
		if now.Sub(c.lastUsed) > 90*time.Second {
			if c.client != nil {
				_ = c.client.Close()
			}
			delete(sshPool, k)
		}
		c.mu.Unlock()
	}
}

// FindInstance 根据 instance_id 或 IP 从资产缓存中智能定位实例
func FindInstance(idOrIP string) (*providers.Instance, error) {
	trimmed := strings.TrimSpace(idOrIP)
	if trimmed == "" {
		return nil, errors.New("instance id or IP cannot be empty")
	}

	cache, err := store.LoadDailyCache()
	if err != nil || cache == nil {
		return nil, fmt.Errorf("failed to load instances daily cache: %w", err)
	}

	// 1. 精确匹配 ID
	for _, inst := range cache.Instances {
		if inst.ID == trimmed {
			cpy := inst
			fillDefaults(&cpy)
			return &cpy, nil
		}
	}

	// 2. 匹配 IP 或 IPv4 / IPv6
	for _, inst := range cache.Instances {
		if inst.IP == trimmed || inst.IPv4 == trimmed || inst.IPv6 == trimmed {
			cpy := inst
			fillDefaults(&cpy)
			return &cpy, nil
		}
	}

	// 3. 忽略大小写或部分前缀匹配 (例如用户输入 "lax" 或带有空格)
	for _, inst := range cache.Instances {
		if strings.EqualFold(inst.Name, trimmed) || strings.Contains(strings.ToLower(inst.Name), strings.ToLower(trimmed)) {
			cpy := inst
			fillDefaults(&cpy)
			return &cpy, nil
		}
	}

	return nil, fmt.Errorf("instance not found for identifier: %s", idOrIP)
}

func fillDefaults(inst *providers.Instance) {
	if inst.SSHPort <= 0 && inst.ProviderID != "lazycat" {
		inst.SSHPort = 22
	}
	if inst.SSHUser == "" {
		inst.SSHUser = "root"
	}
	if inst.SSHPassword == "" && inst.RootPassword != "" {
		inst.SSHPassword = inst.RootPassword
	}
	if inst.IP == "" && inst.IPv4 != "" {
		inst.IP = inst.IPv4
	}

	if inst.SSHPassword == "" {
		if cache, err := store.LoadDailyCache(); err == nil && cache != nil {
			for _, cached := range cache.Instances {
				if cached.IP == inst.IP || cached.ID == inst.ID {
					if cached.SSHPassword != "" {
						inst.SSHPassword = cached.SSHPassword
						break
					}
					if cached.RootPassword != "" {
						inst.SSHPassword = cached.RootPassword
						break
					}
				}
			}
		}
	}
}

// getSSHClient 获取或复用 SSH 连接 (5~8 秒拨号超时)
func getSSHClient(inst *providers.Instance) (*ssh.Client, error) {
	initCleaner()

	hostKey := fmt.Sprintf("%s:%d", inst.IP, inst.SSHPort)

	poolMu.Lock()
	cached, ok := sshPool[hostKey]
	if !ok {
		cached = &cachedSSHClient{hostKey: hostKey}
		sshPool[hostKey] = cached
	}
	poolMu.Unlock()

	cached.mu.Lock()
	defer cached.mu.Unlock()

	// 检查旧连接是否仍然健康
	if cached.client != nil {
		// 发送 keepalive 测试连接
		_, _, err := cached.client.SendRequest("keepalive@openssh.com", true, nil)
		if err == nil {
			cached.lastUsed = time.Now()
			return cached.client, nil
		}
		// 旧连接已断，关闭并重拨
		_ = cached.client.Close()
		cached.client = nil
	}

	// 建立新连接
	sshConfig := &ssh.ClientConfig{
		User: inst.SSHUser,
		Auth: []ssh.AuthMethod{
			ssh.Password(inst.SSHPassword),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         8 * time.Second,
	}

	addr := net.JoinHostPort(inst.IP, fmt.Sprintf("%d", inst.SSHPort))
	client, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		return nil, fmt.Errorf("ssh dial to %s failed: %w", addr, err)
	}

	cached.client = client
	cached.lastUsed = time.Now()
	return client, nil
}

// InvalidateClient 主动使连接缓存失效
func InvalidateClient(ip string, port int) {
	if port <= 0 {
		port = 22
	}
	hostKey := fmt.Sprintf("%s:%d", ip, port)

	poolMu.Lock()
	defer poolMu.Unlock()

	if cached, ok := sshPool[hostKey]; ok {
		cached.mu.Lock()
		if cached.client != nil {
			_ = cached.client.Close()
			cached.client = nil
		}
		cached.mu.Unlock()
		delete(sshPool, hostKey)
	}
}

// ExecuteCommand 在指定实例上执行免交互命令（懒猫 NAT 容器优先走 WebSocket PTY，支持 22 端口映射兜底）
func ExecuteCommand(ctx context.Context, inst *providers.Instance, command string, timeout time.Duration) (*ExecResult, error) {
	fillDefaults(inst)
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if timeout > 60*time.Second {
		timeout = 60 * time.Second
	}

	// 懒猫云 NAT 容器：优先使用官方 WebSocket PTY 通道（不消耗宝贵的 NAT 端口配额）
	if inst.ProviderID == "lazycat" {
		res, available, err := lazyCatExecutorFn(ctx, inst.ID, command, timeout)
		if available {
			if err == nil {
				return &ExecResult{
					ExitCode:   res.ExitCode,
					Stdout:     res.Output,
					Stderr:     "",
					DurationMs: res.DurationMs,
				}, nil
			}
			// 若未映射公网 22 端口，直接返回 WebSocket PTY 错误，避免误拨 22 端口
			if inst.SSHPort <= 0 || inst.SSHPassword == "" {
				return nil, err
			}
		} else if inst.SSHPort <= 0 {
			return nil, fmt.Errorf("lazycat provider unavailable and instance %s has no mapped SSH port", inst.ID)
		}
	}

	if inst.IP == "" {
		return nil, errors.New("instance IP is empty")
	}
	if inst.SSHPort <= 0 {
		return nil, fmt.Errorf("instance %s (%s) has no mapped public SSH port", inst.ID, inst.IP)
	}
	if inst.SSHPassword == "" {
		return nil, fmt.Errorf("instance %s (%s) has no SSH password/root password configured", inst.ID, inst.IP)
	}

	start := time.Now()

	client, err := getSSHClient(inst)
	if err != nil {
		return nil, err
	}

	session, err := client.NewSession()
	if err != nil {
		// Session 创建失败可能因为连接失效，清除缓存重试一次
		InvalidateClient(inst.IP, inst.SSHPort)
		client, err = getSSHClient(inst)
		if err != nil {
			return nil, fmt.Errorf("retry ssh dial failed: %w", err)
		}
		session, err = client.NewSession()
		if err != nil {
			return nil, fmt.Errorf("failed to create ssh session: %w", err)
		}
	}
	defer session.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	session.Stdout = &stdoutBuf
	session.Stderr = &stderrBuf

	type runOutcome struct {
		err error
	}
	done := make(chan runOutcome, 1)

	go func() {
		done <- runOutcome{err: session.Run(command)}
	}()

	var runErr error
	select {
	case <-ctx.Done():
		_ = session.Signal(ssh.SIGKILL)
		return nil, ctx.Err()
	case <-time.After(timeout):
		_ = session.Signal(ssh.SIGKILL)
		return nil, fmt.Errorf("command execution timed out after %v", timeout)
	case out := <-done:
		runErr = out.err
	}

	durMs := time.Since(start).Milliseconds()

	exitCode := 0
	if runErr != nil {
		var exitErr *ssh.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitStatus()
		} else {
			// 非正常退出错误 (如断连)
			InvalidateClient(inst.IP, inst.SSHPort)
			return nil, fmt.Errorf("ssh session run error: %w", runErr)
		}
	}

	return &ExecResult{
		ExitCode:   exitCode,
		Stdout:     stdoutBuf.String(),
		Stderr:     stderrBuf.String(),
		DurationMs: durMs,
	}, nil
}

// RunCommandWithAudit 执行命令并自动记录审计日志
func RunCommandWithAudit(ctx context.Context, source, idOrIP, command string, timeoutSec int) (*ExecResult, *providers.Instance, error) {
	inst, err := FindInstance(idOrIP)
	if err != nil {
		dur := int64(0)
		audit.Record(source, "vps.exec", "system", map[string]interface{}{
			"id_or_ip": idOrIP,
			"command":  command,
		}, "FAILED", fmt.Sprintf("Instance not found: %v", err), dur)
		return nil, nil, err
	}

	timeout := time.Duration(timeoutSec) * time.Second
	if timeoutSec <= 0 {
		timeout = 10 * time.Second
	}

	res, execErr := ExecuteCommand(ctx, inst, command, timeout)
	if execErr != nil {
		audit.Record(source, "vps.exec", inst.ProviderID, map[string]interface{}{
			"instance_id": inst.ID,
			"ip":          inst.IP,
			"command":     command,
			"timeout_sec": timeoutSec,
		}, "FAILED", execErr.Error(), 0)
		return nil, inst, execErr
	}

	status := "SUCCESS"
	if res.ExitCode != 0 {
		status = "FAILED"
	}

	msg := fmt.Sprintf("exit=%d stdout=%d bytes stderr=%d bytes", res.ExitCode, len(res.Stdout), len(res.Stderr))
	audit.Record(source, "vps.exec", inst.ProviderID, map[string]interface{}{
		"instance_id": inst.ID,
		"ip":          inst.IP,
		"command":     command,
		"exit_code":   res.ExitCode,
	}, status, msg, res.DurationMs)

	return res, inst, nil
}

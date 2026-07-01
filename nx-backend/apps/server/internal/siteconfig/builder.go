package siteconfig

import (
	"bytes"
	"context"
	"os/exec"
	"sync"
	"time"
)

// BuildState 描述一次官网构建的生命周期状态。
type BuildState string

const (
	BuildIdle     BuildState = "idle"
	BuildPending  BuildState = "pending" // 已排队，等待空闲槽位
	BuildRunning  BuildState = "building"
	BuildSuccess  BuildState = "success"
	BuildFailed   BuildState = "failed"
	BuildDisabled BuildState = "disabled" // 未配置构建脚本
)

// BuildStatus 是返回给前端轮询的快照。
type BuildStatus struct {
	State      BuildState `json:"state"`
	StartedAt  string     `json:"startedAt"`
	FinishedAt string     `json:"finishedAt"`
	DurationMS int64      `json:"durationMs"`
	Message    string     `json:"message"`
	Log        string     `json:"log"`
	QueuedNext bool       `json:"queuedNext"` // 当前构建期间又收到了新的保存请求
}

// Builder 串行执行官网构建：同一时刻最多一个构建在跑；
// 构建期间到来的多次 Trigger 会被合并成一次「待构建」，
// 当前构建结束后自动再跑一次，确保最终产物基于最新配置。
type Builder struct {
	script  string
	workDir string
	timeout time.Duration

	mu      sync.Mutex
	status  BuildStatus
	running bool
	queued  bool
}

// NewBuilder 创建一个构建器。script 为空时 Builder 进入 disabled 状态，
// Trigger 变为无操作（适合本地纯调试、不需要自动发布的场景）。
func NewBuilder(script, workDir string, timeout time.Duration) *Builder {
	state := BuildIdle
	msg := ""
	if script == "" {
		state = BuildDisabled
		msg = "BUILD_SCRIPT 未配置，已跳过自动构建"
	}
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	return &Builder{
		script:  script,
		workDir: workDir,
		timeout: timeout,
		status:  BuildStatus{State: state, Message: msg},
	}
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006/01/02 15:04:05")
}

// Status 返回当前构建状态快照。
func (b *Builder) Status() BuildStatus {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.status
}

// Trigger 请求一次构建。若已有构建在跑，则标记 queued，
// 当前构建结束后会自动补跑一次。非阻塞。
func (b *Builder) Trigger() {
	if b.script == "" {
		return
	}

	b.mu.Lock()
	if b.running {
		b.queued = true
		b.status.QueuedNext = true
		b.mu.Unlock()
		return
	}
	b.running = true
	b.status = BuildStatus{State: BuildPending, Message: "已排队"}
	b.mu.Unlock()

	go b.loop()
}

func (b *Builder) loop() {
	for {
		b.runOnce()

		b.mu.Lock()
		if b.queued {
			b.queued = false
			b.status.QueuedNext = false
			b.mu.Unlock()
			continue
		}
		b.running = false
		b.mu.Unlock()
		return
	}
}

func (b *Builder) runOnce() {
	start := time.Now()

	b.mu.Lock()
	b.status = BuildStatus{
		State:     BuildRunning,
		StartedAt: formatTime(start),
		Message:   "正在构建官网…",
	}
	b.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/bin/sh", b.script)
	if b.workDir != "" {
		cmd.Dir = b.workDir
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()

	finish := time.Now()
	logText := tail(out.String(), 8000)

	b.mu.Lock()
	defer b.mu.Unlock()
	b.status.StartedAt = formatTime(start)
	b.status.FinishedAt = formatTime(finish)
	b.status.DurationMS = finish.Sub(start).Milliseconds()
	b.status.Log = logText
	if err != nil {
		b.status.State = BuildFailed
		if ctx.Err() == context.DeadlineExceeded {
			b.status.Message = "构建超时"
		} else {
			b.status.Message = "构建失败：" + err.Error()
		}
		return
	}
	b.status.State = BuildSuccess
	b.status.Message = "构建成功，官网已更新"
}

// tail 截取字符串尾部 max 字节，避免日志过大撑爆响应。
func tail(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return "…(truncated)…\n" + s[len(s)-max:]
}

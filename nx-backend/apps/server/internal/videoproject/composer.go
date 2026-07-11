// Package videoproject 视频合成器：使用 FFmpeg 拼接多个视频
package videoproject

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Composer 视频合成器
type Composer struct {
	tempDir    string
	uploadRoot string
}

// NewComposer 创建合成器
func NewComposer(tempDir string, uploadRoots ...string) *Composer {
	if tempDir == "" {
		tempDir = os.TempDir()
	}
	uploadRoot := tempDir
	if len(uploadRoots) > 0 && strings.TrimSpace(uploadRoots[0]) != "" {
		uploadRoot = uploadRoots[0]
	}
	return &Composer{tempDir: tempDir, uploadRoot: uploadRoot}
}

// ComposeOptions 合成选项
type ComposeOptions struct {
	Transition      string  // 转场效果: none, fade, wipeleft, circleopen
	MusicURL        string  // 背景音乐 URL
	EnableSubtitles bool    // 是否添加字幕
	TransitionDur   float64 // 转场时长（秒），默认1.0
}

// ComposeResult 合成结果
type ComposeResult struct {
	OutputPath string  // 输出文件路径
	Duration   float64 // 总时长（秒）
	FileSize   int64   // 文件大小（字节）
}

// ComposeVideos 合成多个视频
func (c *Composer) ComposeVideos(ctx context.Context, videoURLs []string, opts ComposeOptions) (ComposeResult, error) {
	if len(videoURLs) == 0 {
		return ComposeResult{}, fmt.Errorf("视频列表为空")
	}

	// 1. 下载所有视频到本地
	localPaths, err := c.downloadVideos(ctx, videoURLs)
	if err != nil {
		return ComposeResult{}, fmt.Errorf("下载视频失败: %v", err)
	}
	defer c.cleanupFiles(localPaths...)

	// 2. 根据选项选择合成方式
	var outputPath string
	if opts.Transition != "" && opts.Transition != "none" {
		// 有转场效果
		outputPath, err = c.composeWithTransition(ctx, localPaths, opts)
	} else {
		// 简单拼接（最快）
		outputPath, err = c.composeSimple(ctx, localPaths)
	}
	if err != nil {
		return ComposeResult{}, err
	}

	// 3. 如果需要添加背景音乐
	if opts.MusicURL != "" {
		outputPath, err = c.addBackgroundMusic(ctx, outputPath, opts.MusicURL)
		if err != nil {
			os.Remove(outputPath)
			return ComposeResult{}, fmt.Errorf("添加背景音乐失败: %v", err)
		}
	}

	// 4. 获取输出文件信息
	stat, err := os.Stat(outputPath)
	if err != nil {
		return ComposeResult{}, fmt.Errorf("获取文件信息失败: %v", err)
	}

	duration, _ := c.getVideoDuration(ctx, outputPath)

	return ComposeResult{
		OutputPath: outputPath,
		Duration:   duration,
		FileSize:   stat.Size(),
	}, nil
}

// composeSimple 简单拼接（无转场，最快）
func (c *Composer) composeSimple(ctx context.Context, videoPaths []string) (string, error) {
	// 1. 创建 concat 文件列表
	concatFile := filepath.Join(c.tempDir, fmt.Sprintf("concat_%d.txt", time.Now().Unix()))
	content := ""
	for _, path := range videoPaths {
		// FFmpeg concat 需要绝对路径
		absPath, _ := filepath.Abs(path)
		content += fmt.Sprintf("file '%s'\n", absPath)
	}
	if err := os.WriteFile(concatFile, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("创建 concat 文件失败: %v", err)
	}
	defer os.Remove(concatFile)

	// 2. 执行 FFmpeg concat
	outputPath := filepath.Join(c.tempDir, fmt.Sprintf("composed_%d.mp4", time.Now().Unix()))
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-f", "concat",
		"-safe", "0",
		"-i", concatFile,
		"-c", "copy", // 直接复制流，最快
		"-y",
		outputPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("FFmpeg concat 失败: %v, 输出: %s", err, string(output))
	}

	return outputPath, nil
}

// composeWithTransition 带转场效果的拼接
func (c *Composer) composeWithTransition(ctx context.Context, videoPaths []string, opts ComposeOptions) (string, error) {
	if len(videoPaths) < 2 {
		// 只有一个视频，无需转场
		return c.composeSimple(ctx, videoPaths)
	}

	transitionDur := opts.TransitionDur
	if transitionDur <= 0 {
		transitionDur = 1.0
	}

	// 构建 xfade 滤镜
	// 参考: https://trac.ffmpeg.org/wiki/Xfade
	filterComplex := c.buildXfadeFilter(ctx, videoPaths, opts.Transition, transitionDur)

	// 构建输入参数
	inputArgs := []string{}
	for _, path := range videoPaths {
		inputArgs = append(inputArgs, "-i", path)
	}

	outputPath := filepath.Join(c.tempDir, fmt.Sprintf("composed_%d.mp4", time.Now().Unix()))

	args := append(inputArgs,
		"-filter_complex", filterComplex,
		"-map", fmt.Sprintf("[v%d]", len(videoPaths)-1), // 映射最后的视频流
		"-c:v", "libx264",
		"-preset", "medium",
		"-crf", "23",
		"-y",
		outputPath,
	)

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("FFmpeg xfade 失败: %v, 输出: %s", err, string(output))
	}

	return outputPath, nil
}

// buildXfadeFilter 构建 xfade 转场滤镜
func (c *Composer) buildXfadeFilter(ctx context.Context, videoPaths []string, transition string, duration float64) string {
	// 获取每个视频的时长
	durations := make([]float64, len(videoPaths))
	for i, path := range videoPaths {
		dur, err := c.getVideoDuration(ctx, path)
		if err != nil {
			dur = 15.0 // 默认15秒
		}
		durations[i] = dur
	}

	// 构建滤镜链
	filter := ""
	offset := 0.0

	for i := 0; i < len(videoPaths)-1; i++ {
		offset += durations[i] - duration // 转场开始时间点

		if i == 0 {
			// 第一个转场
			filter += fmt.Sprintf("[0:v][1:v]xfade=transition=%s:duration=%.2f:offset=%.2f[v1];",
				transition, duration, offset)
		} else {
			// 后续转场
			filter += fmt.Sprintf("[v%d][%d:v]xfade=transition=%s:duration=%.2f:offset=%.2f[v%d];",
				i, i+1, transition, duration, offset, i+1)
		}
	}

	// 去掉最后的分号
	filter = strings.TrimSuffix(filter, ";")
	return filter
}

// addBackgroundMusic 添加背景音乐
func (c *Composer) addBackgroundMusic(ctx context.Context, videoPath, musicURL string) (string, error) {
	// 1. 下载音乐文件
	musicPath, err := c.downloadFile(ctx, musicURL, "music")
	if err != nil {
		return "", fmt.Errorf("下载音乐失败: %v", err)
	}
	defer os.Remove(musicPath)

	// 2. 获取视频时长
	videoDuration, err := c.getVideoDuration(ctx, videoPath)
	if err != nil {
		return "", fmt.Errorf("获取视频时长失败: %v", err)
	}

	// 3. 合成视频和音乐
	outputPath := filepath.Join(c.tempDir, fmt.Sprintf("with_music_%d.mp4", time.Now().Unix()))
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-i", videoPath,
		"-i", musicPath,
		"-t", fmt.Sprintf("%.2f", videoDuration), // 裁剪音乐到视频时长
		"-c:v", "copy", // 视频流直接复制
		"-c:a", "aac", // 音频重新编码
		"-b:a", "128k",
		"-map", "0:v:0", // 视频流
		"-map", "1:a:0", // 音频流
		"-shortest", // 以最短的流为准
		"-y",
		outputPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("FFmpeg 添加音乐失败: %v, 输出: %s", err, string(output))
	}

	// 删除原视频文件
	os.Remove(videoPath)

	return outputPath, nil
}

// downloadVideos 下载多个视频文件
func (c *Composer) downloadVideos(ctx context.Context, urls []string) ([]string, error) {
	paths := make([]string, len(urls))
	for i, url := range urls {
		path, err := c.downloadFile(ctx, url, fmt.Sprintf("video_%d", i))
		if err != nil {
			// 清理已下载的文件
			c.cleanupFiles(paths[:i]...)
			return nil, fmt.Errorf("下载视频 %d 失败: %v", i, err)
		}
		paths[i] = path
	}
	return paths, nil
}

// downloadFile 下载单个文件
func (c *Composer) downloadFile(ctx context.Context, url, prefix string) (string, error) {
	if strings.HasPrefix(url, "/api/uploads/") {
		return c.copyLocalUpload(url, prefix)
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP 状态码: %d", resp.StatusCode)
	}

	// 从 URL 推断扩展名
	ext := ".mp4"
	if strings.Contains(url, ".mp3") {
		ext = ".mp3"
	} else if strings.Contains(url, ".wav") {
		ext = ".wav"
	}

	outputPath := filepath.Join(c.tempDir, fmt.Sprintf("%s_%d%s", prefix, time.Now().Unix(), ext))
	file, err := os.Create(outputPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	if _, err := io.Copy(file, resp.Body); err != nil {
		os.Remove(outputPath)
		return "", err
	}

	return outputPath, nil
}

func (c *Composer) copyLocalUpload(rawURL, prefix string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "" || parsed.Host != "" {
		return "", fmt.Errorf("invalid local upload URL")
	}
	const publicPrefix = "/api/uploads/"
	escapedPath := parsed.EscapedPath()
	if !strings.HasPrefix(escapedPath, publicPrefix) {
		return "", fmt.Errorf("invalid local upload URL")
	}
	relativePath, err := url.PathUnescape(strings.TrimPrefix(escapedPath, publicPrefix))
	if err != nil || relativePath == "" || strings.Contains(relativePath, "\\") {
		return "", fmt.Errorf("local upload path escapes upload root")
	}
	cleaned := filepath.Clean(filepath.FromSlash(relativePath))
	if filepath.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("local upload path escapes upload root")
	}

	root, err := filepath.Abs(c.uploadRoot)
	if err != nil {
		return "", err
	}
	source := filepath.Join(root, cleaned)
	withinRoot, err := filepath.Rel(root, source)
	if err != nil || withinRoot == ".." || strings.HasPrefix(withinRoot, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("local upload path escapes upload root")
	}
	info, err := os.Stat(source)
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("local upload does not exist: %s", relativePath)
	}
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("local upload is not a regular file")
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	realSource, err := filepath.EvalSymlinks(source)
	if err != nil {
		return "", err
	}
	withinRealRoot, err := filepath.Rel(realRoot, realSource)
	if err != nil || withinRealRoot == ".." || strings.HasPrefix(withinRealRoot, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("local upload path escapes upload root")
	}

	input, err := os.Open(source)
	if err != nil {
		return "", err
	}
	defer input.Close()
	ext := filepath.Ext(cleaned)
	if ext == "" {
		ext = ".mp4"
	}
	output, err := os.CreateTemp(c.tempDir, prefix+"-*"+ext)
	if err != nil {
		return "", err
	}
	outputPath := output.Name()
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		_ = os.Remove(outputPath)
		return "", err
	}
	if err := output.Close(); err != nil {
		_ = os.Remove(outputPath)
		return "", err
	}
	return outputPath, nil
}

// getVideoDuration 获取视频时长
func (c *Composer) getVideoDuration(ctx context.Context, videoPath string) (float64, error) {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		videoPath,
	)

	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	var duration float64
	if _, err := fmt.Sscanf(strings.TrimSpace(string(output)), "%f", &duration); err != nil {
		return 0, err
	}

	return duration, nil
}

// cleanupFiles 清理临时文件
func (c *Composer) cleanupFiles(paths ...string) {
	for _, path := range paths {
		if path != "" {
			os.Remove(path)
		}
	}
}

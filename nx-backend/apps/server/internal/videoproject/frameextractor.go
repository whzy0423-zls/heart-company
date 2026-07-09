// Package videoproject 视频帧提取器：使用 FFmpeg 提取视频首尾帧
package videoproject

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// FrameExtractor 视频帧提取器
type FrameExtractor struct {
	tempDir string // 临时文件目录
}

// NewFrameExtractor 创建帧提取器
func NewFrameExtractor(tempDir string) *FrameExtractor {
	if tempDir == "" {
		tempDir = os.TempDir()
	}
	return &FrameExtractor{tempDir: tempDir}
}

// ExtractFrames 提取视频的首帧和尾帧
// videoPath: 本地视频文件路径
// 返回：startFramePath, endFramePath, error
func (e *FrameExtractor) ExtractFrames(ctx context.Context, videoPath string) (string, string, error) {
	if _, err := os.Stat(videoPath); err != nil {
		return "", "", fmt.Errorf("视频文件不存在: %v", err)
	}

	// 获取视频时长
	duration, err := e.getVideoDuration(ctx, videoPath)
	if err != nil {
		return "", "", fmt.Errorf("无法获取视频时长: %v", err)
	}

	// 生成输出文件名
	timestamp := time.Now().Unix()
	startFramePath := filepath.Join(e.tempDir, fmt.Sprintf("frame_start_%d.jpg", timestamp))
	endFramePath := filepath.Join(e.tempDir, fmt.Sprintf("frame_end_%d.jpg", timestamp))

	// 提取首帧（0.1秒处）
	if err := e.extractFrame(ctx, videoPath, 0.1, startFramePath); err != nil {
		return "", "", fmt.Errorf("提取首帧失败: %v", err)
	}

	// 提取尾帧（倒数0.5秒处，避免黑场）
	endTime := duration - 0.5
	if endTime < 0.1 {
		endTime = duration * 0.9 // 如果视频很短，取90%处
	}
	if err := e.extractFrame(ctx, videoPath, endTime, endFramePath); err != nil {
		// 首帧已提取成功，清理后返回错误
		os.Remove(startFramePath)
		return "", "", fmt.Errorf("提取尾帧失败: %v", err)
	}

	return startFramePath, endFramePath, nil
}

// ExtractStartFrame 仅提取首帧
func (e *FrameExtractor) ExtractStartFrame(ctx context.Context, videoPath string) (string, error) {
	if _, err := os.Stat(videoPath); err != nil {
		return "", fmt.Errorf("视频文件不存在: %v", err)
	}

	timestamp := time.Now().Unix()
	outputPath := filepath.Join(e.tempDir, fmt.Sprintf("frame_start_%d.jpg", timestamp))

	if err := e.extractFrame(ctx, videoPath, 0.1, outputPath); err != nil {
		return "", fmt.Errorf("提取首帧失败: %v", err)
	}

	return outputPath, nil
}

// ExtractEndFrame 仅提取尾帧
func (e *FrameExtractor) ExtractEndFrame(ctx context.Context, videoPath string) (string, error) {
	if _, err := os.Stat(videoPath); err != nil {
		return "", fmt.Errorf("视频文件不存在: %v", err)
	}

	duration, err := e.getVideoDuration(ctx, videoPath)
	if err != nil {
		return "", fmt.Errorf("无法获取视频时长: %v", err)
	}

	timestamp := time.Now().Unix()
	outputPath := filepath.Join(e.tempDir, fmt.Sprintf("frame_end_%d.jpg", timestamp))

	endTime := duration - 0.5
	if endTime < 0.1 {
		endTime = duration * 0.9
	}

	if err := e.extractFrame(ctx, videoPath, endTime, outputPath); err != nil {
		return "", fmt.Errorf("提取尾帧失败: %v", err)
	}

	return outputPath, nil
}

// ExtractFrameAtTime 提取指定秒数的一帧，供视频版本“视频抽帧”写回分镜参考素材。
func (e *FrameExtractor) ExtractFrameAtTime(ctx context.Context, videoPath string, second float64) (string, error) {
	if _, err := os.Stat(videoPath); err != nil {
		return "", fmt.Errorf("视频文件不存在: %v", err)
	}
	if second < 0 {
		second = 0.1
	}
	if second == 0 {
		second = 0.1
	}

	timestamp := time.Now().UnixNano()
	outputPath := filepath.Join(e.tempDir, fmt.Sprintf("frame_extract_%d.jpg", timestamp))
	if err := e.extractFrame(ctx, videoPath, second, outputPath); err != nil {
		return "", fmt.Errorf("提取视频帧失败: %v", err)
	}
	return outputPath, nil
}

// extractFrame 使用 FFmpeg 提取指定时间点的帧
func (e *FrameExtractor) extractFrame(ctx context.Context, videoPath string, timestamp float64, outputPath string) error {
	// ffmpeg -ss {timestamp} -i {video} -frames:v 1 -q:v 2 {output}
	// -ss: 指定时间点
	// -frames:v 1: 只提取1帧
	// -q:v 2: 高质量（1-31，数字越小质量越高）
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-ss", fmt.Sprintf("%.2f", timestamp),
		"-i", videoPath,
		"-frames:v", "1",
		"-q:v", "2",
		"-y", // 覆盖已存在的文件
		outputPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("FFmpeg 执行失败: %v, 输出: %s", err, string(output))
	}

	// 验证文件是否生成
	if _, err := os.Stat(outputPath); err != nil {
		return fmt.Errorf("帧文件未生成: %v", err)
	}

	return nil
}

// getVideoDuration 获取视频时长（秒）
func (e *FrameExtractor) getVideoDuration(ctx context.Context, videoPath string) (float64, error) {
	// ffprobe -v error -show_entries format=duration -of default=noprint_wrappers=1:nokey=1 {video}
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		videoPath,
	)

	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("FFprobe 执行失败: %v", err)
	}

	var duration float64
	if _, err := fmt.Sscanf(strings.TrimSpace(string(output)), "%f", &duration); err != nil {
		return 0, fmt.Errorf("解析视频时长失败: %v", err)
	}

	return duration, nil
}

// CleanupFrames 清理临时帧文件
func (e *FrameExtractor) CleanupFrames(framePaths ...string) {
	for _, path := range framePaths {
		if path != "" {
			os.Remove(path)
		}
	}
}

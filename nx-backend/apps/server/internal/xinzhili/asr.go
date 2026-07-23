package xinzhili

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrASRClosed        = errors.New("实时语音识别会话已关闭")
	ErrASREmptyPCM      = errors.New("实时语音识别音频帧不能为空")
	ErrASRInputFinished = errors.New("实时语音识别输入已经结束")
	ErrASRProtocol      = errors.New("实时语音识别上游协议错误")
	ErrASRTimeout       = errors.New("实时语音识别上游超时")
	ErrASRUpstream      = errors.New("实时语音识别上游失败")
	ErrASRDisconnected  = errors.New("实时语音识别上游连接中断")
)

type ASREventKind string

const (
	ASREventSpeechStarted ASREventKind = "speech_started"
	ASREventPartial       ASREventKind = "partial"
	ASREventFinal         ASREventKind = "final"
	ASREventTaskFinished  ASREventKind = "task_finished"
)

type ASREvent struct {
	Kind    ASREventKind
	Partial string
	Final   string
	Stable  bool
	TaskID  string
	At      time.Time
}

type ASRFactory interface {
	Open(ctx context.Context, cfg RealtimeASRConfig) (ASRSession, error)
}

type ASRSession interface {
	WritePCM(ctx context.Context, pcm []byte) error
	FinishInput(ctx context.Context) error
	Events() <-chan ASREvent
	Err() error
	Close() error
}

type ASRUpstreamError struct {
	Code    string
	Message string
}

func (e *ASRUpstreamError) Error() string {
	if e.Code == "" {
		return ErrASRUpstream.Error()
	}
	return fmt.Sprintf("%s: %s", ErrASRUpstream, e.Code)
}

func (e *ASRUpstreamError) Unwrap() error { return ErrASRUpstream }

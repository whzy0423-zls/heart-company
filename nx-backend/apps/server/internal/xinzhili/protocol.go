package xinzhili

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const (
	ProtocolVersion = "xinzhili.voice.v1"

	BinaryHeaderLength     = 32
	MaxBinaryPayloadLength = int64(^uint32(0))
)

var ErrInvalidBinaryFrame = errors.New("invalid xinzhili binary frame")

const (
	ProtocolErrorInvalidEnvelope       = "invalid_envelope"
	ProtocolErrorUnsupportedVersion    = "unsupported_version"
	ProtocolErrorInvalidEventDirection = "invalid_event_direction"
	ProtocolErrorInvalidPayload        = "invalid_payload"
)

type ProtocolError struct {
	Code string
	Err  error
}

func (err *ProtocolError) Error() string {
	return err.Code + ": " + err.Err.Error()
}

func (err *ProtocolError) Unwrap() error {
	return err.Err
}

type Direction string

const (
	DirectionClient Direction = "client"
	DirectionServer Direction = "server"
)

type EventType string

const (
	EventSessionStart         EventType = "session.start"
	EventSessionStop          EventType = "session.stop"
	EventSessionPing          EventType = "session.ping"
	EventTurnStart            EventType = "turn.start"
	EventTurnCancel           EventType = "turn.cancel"
	EventModeChange           EventType = "mode.change"
	EventPlaybackInterrupt    EventType = "playback.interrupt"
	EventAssistantPlaybackAck EventType = "assistant.playback_ack"
	EventSessionReady         EventType = "session.ready"
	EventSessionPong          EventType = "session.pong"
	EventConfigChanged        EventType = "session.config_changed"
	EventModeChanged          EventType = "session.mode_changed"
	EventASRActivity          EventType = "asr.activity"
	EventTurnProcessing       EventType = "turn.processing"
	EventTurnCancelled        EventType = "turn.cancelled"
	EventAssistantAudioStart  EventType = "assistant.audio_start"
	EventAssistantAudioEnd    EventType = "assistant.audio_end"
	EventAssistantDone        EventType = "assistant.done"
	EventError                EventType = "error"
)

type Envelope struct {
	ProtocolVersion string          `json:"protocolVersion"`
	Type            EventType       `json:"type"`
	SessionID       *string         `json:"sessionId"`
	Generation      uint32          `json:"generation"`
	TurnID          *string         `json:"turnId"`
	SessionSeq      *uint64         `json:"sessionSeq"`
	TurnSeq         *uint64         `json:"turnSeq"`
	ConfigVersion   int64           `json:"configVersion"`
	TimestampMs     int64           `json:"timestampMs"`
	Payload         json.RawMessage `json:"payload"`
}

type ErrorPayload struct {
	Code         string `json:"code"`
	Message      string `json:"message"`
	Retryable    bool   `json:"retryable"`
	Fatal        bool   `json:"fatal"`
	RetryAfterMs *int64 `json:"retryAfterMs,omitempty"`
}

func EncodeEnvelope(envelope Envelope, direction Direction, sessionReady bool) ([]byte, error) {
	if err := ValidateEnvelope(envelope, direction, sessionReady); err != nil {
		return nil, err
	}
	return json.Marshal(envelope)
}

func DecodeEnvelope(data []byte, direction Direction, sessionReady bool) (Envelope, error) {
	var wire wireEnvelope
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return Envelope{}, newProtocolError(ProtocolErrorInvalidEnvelope, fmt.Errorf("decode envelope: %w", err))
	}
	if err := requireJSONEOF(decoder); err != nil {
		return Envelope{}, newProtocolError(ProtocolErrorInvalidEnvelope, err)
	}
	envelope, err := wire.envelope()
	if err != nil {
		return Envelope{}, err
	}
	if err := ValidateEnvelope(envelope, direction, sessionReady); err != nil {
		return Envelope{}, err
	}
	return envelope, nil
}

func ValidateEnvelope(envelope Envelope, direction Direction, sessionReady bool) error {
	if envelope.ProtocolVersion != ProtocolVersion {
		return newProtocolError(ProtocolErrorUnsupportedVersion, fmt.Errorf("unsupported protocolVersion %q", envelope.ProtocolVersion))
	}
	eventLevel, err := validateEventDirection(envelope.Type, direction)
	if err != nil {
		return newProtocolError(ProtocolErrorInvalidEventDirection, err)
	}
	if envelope.SessionSeq == nil {
		return newProtocolError(ProtocolErrorInvalidEnvelope, errors.New("sessionSeq is required"))
	}
	if !isJSONObject(envelope.Payload) {
		return newProtocolError(ProtocolErrorInvalidPayload, errors.New("payload must be a JSON object"))
	}

	if envelope.Type == EventSessionStart {
		if sessionReady {
			return newProtocolError(ProtocolErrorInvalidEnvelope, errors.New("session.start is only valid before session.ready"))
		}
		if envelope.SessionID != nil {
			return newProtocolError(ProtocolErrorInvalidEnvelope, errors.New("session.start requires null sessionId"))
		}
	}

	requiresSessionID := sessionReady || envelope.Type == EventSessionReady
	if requiresSessionID && (envelope.SessionID == nil || *envelope.SessionID == "") {
		return newProtocolError(ProtocolErrorInvalidEnvelope, errors.New("sessionId is required"))
	}
	if envelope.SessionID != nil && *envelope.SessionID == "" {
		return newProtocolError(ProtocolErrorInvalidEnvelope, errors.New("sessionId cannot be empty"))
	}

	if eventLevel == eventLevelSession {
		if envelope.TurnID != nil || envelope.TurnSeq != nil {
			return newProtocolError(ProtocolErrorInvalidEnvelope, errors.New("session event requires null turnId and turnSeq"))
		}
	} else {
		if envelope.TurnID == nil || *envelope.TurnID == "" || envelope.TurnSeq == nil {
			return newProtocolError(ProtocolErrorInvalidEnvelope, errors.New("turn event requires turnId and turnSeq"))
		}
	}

	if envelope.Type == EventTurnStart {
		if err := validateTurnStartPayload(envelope); err != nil {
			return newProtocolError(ProtocolErrorInvalidPayload, err)
		}
	}
	if envelope.Type == EventError {
		if err := validateErrorPayload(envelope.Payload); err != nil {
			return newProtocolError(ProtocolErrorInvalidPayload, err)
		}
	}
	return nil
}

type wireEnvelope struct {
	ProtocolVersion json.RawMessage `json:"protocolVersion"`
	Type            json.RawMessage `json:"type"`
	SessionID       json.RawMessage `json:"sessionId"`
	Generation      json.RawMessage `json:"generation"`
	TurnID          json.RawMessage `json:"turnId"`
	SessionSeq      json.RawMessage `json:"sessionSeq"`
	TurnSeq         json.RawMessage `json:"turnSeq"`
	ConfigVersion   json.RawMessage `json:"configVersion"`
	TimestampMs     json.RawMessage `json:"timestampMs"`
	Payload         json.RawMessage `json:"payload"`
}

func (wire wireEnvelope) envelope() (Envelope, error) {
	var envelope Envelope
	if err := decodeRequired(wire.ProtocolVersion, "protocolVersion", &envelope.ProtocolVersion); err != nil {
		return Envelope{}, err
	}
	if err := decodeRequired(wire.Type, "type", &envelope.Type); err != nil {
		return Envelope{}, err
	}
	if err := decodeOptional(wire.SessionID, "sessionId", &envelope.SessionID); err != nil {
		return Envelope{}, err
	}
	if err := decodeRequired(wire.Generation, "generation", &envelope.Generation); err != nil {
		return Envelope{}, err
	}
	if err := decodeOptional(wire.TurnID, "turnId", &envelope.TurnID); err != nil {
		return Envelope{}, err
	}
	if err := decodeRequiredPointer(wire.SessionSeq, "sessionSeq", &envelope.SessionSeq); err != nil {
		return Envelope{}, err
	}
	if err := decodeOptional(wire.TurnSeq, "turnSeq", &envelope.TurnSeq); err != nil {
		return Envelope{}, err
	}
	if err := decodeRequired(wire.ConfigVersion, "configVersion", &envelope.ConfigVersion); err != nil {
		return Envelope{}, err
	}
	if err := decodeRequired(wire.TimestampMs, "timestampMs", &envelope.TimestampMs); err != nil {
		return Envelope{}, err
	}
	if len(wire.Payload) == 0 || isJSONNull(wire.Payload) {
		return Envelope{}, newProtocolError(ProtocolErrorInvalidPayload, errors.New("payload is required and cannot be null"))
	}
	envelope.Payload = bytes.Clone(wire.Payload)
	return envelope, nil
}

func decodeRequired[T any](raw json.RawMessage, name string, destination *T) error {
	if len(raw) == 0 || isJSONNull(raw) {
		return newProtocolError(ProtocolErrorInvalidEnvelope, fmt.Errorf("%s is required and cannot be null", name))
	}
	if err := json.Unmarshal(raw, destination); err != nil {
		return newProtocolError(ProtocolErrorInvalidEnvelope, fmt.Errorf("decode %s: %w", name, err))
	}
	return nil
}

func decodeRequiredPointer[T any](raw json.RawMessage, name string, destination **T) error {
	var value T
	if err := decodeRequired(raw, name, &value); err != nil {
		return err
	}
	*destination = &value
	return nil
}

func decodeOptional[T any](raw json.RawMessage, name string, destination **T) error {
	if len(raw) == 0 || isJSONNull(raw) {
		*destination = nil
		return nil
	}
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		return newProtocolError(ProtocolErrorInvalidEnvelope, fmt.Errorf("decode %s: %w", name, err))
	}
	*destination = &value
	return nil
}

func isJSONNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func isJSONObject(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) >= 2 && trimmed[0] == '{' && trimmed[len(trimmed)-1] == '}' && json.Valid(trimmed)
}

func newProtocolError(code string, err error) error {
	var protocolErr *ProtocolError
	if errors.As(err, &protocolErr) {
		return err
	}
	return &ProtocolError{Code: code, Err: err}
}

type eventLevel uint8

const (
	eventLevelSession eventLevel = iota
	eventLevelTurn
)

func validateEventDirection(eventType EventType, direction Direction) (eventLevel, error) {
	var events map[EventType]eventLevel
	switch direction {
	case DirectionClient:
		events = clientEvents
	case DirectionServer:
		events = serverEvents
	default:
		return 0, fmt.Errorf("unknown direction %q", direction)
	}
	level, ok := events[eventType]
	if !ok {
		return 0, fmt.Errorf("event %q is not valid for %s direction", eventType, direction)
	}
	return level, nil
}

var clientEvents = map[EventType]eventLevel{
	EventSessionStart:         eventLevelSession,
	EventSessionStop:          eventLevelSession,
	EventSessionPing:          eventLevelSession,
	EventTurnStart:            eventLevelTurn,
	EventTurnCancel:           eventLevelTurn,
	EventModeChange:           eventLevelSession,
	EventPlaybackInterrupt:    eventLevelTurn,
	EventAssistantPlaybackAck: eventLevelTurn,
}

var serverEvents = map[EventType]eventLevel{
	EventSessionReady:        eventLevelSession,
	EventSessionPong:         eventLevelSession,
	EventConfigChanged:       eventLevelSession,
	EventModeChanged:         eventLevelSession,
	EventASRActivity:         eventLevelTurn,
	EventTurnProcessing:      eventLevelTurn,
	EventTurnCancelled:       eventLevelTurn,
	EventAssistantAudioStart: eventLevelTurn,
	EventAssistantAudioEnd:   eventLevelTurn,
	EventAssistantDone:       eventLevelTurn,
	EventError:               eventLevelSession,
}

func validateTurnStartPayload(envelope Envelope) error {
	var payload struct {
		TurnKey *uint64 `json:"turnKey"`
	}
	decoder := json.NewDecoder(bytes.NewReader(envelope.Payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return fmt.Errorf("decode turn.start payload: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return err
	}
	if payload.TurnKey == nil {
		return errors.New("turn.start payload requires turnKey")
	}
	if *payload.TurnKey != TurnKey(*envelope.TurnID) {
		return errors.New("turn.start turnKey does not match turnId")
	}
	return nil
}

func validateErrorPayload(raw json.RawMessage) error {
	var payload struct {
		Code         string `json:"code"`
		Message      string `json:"message"`
		Retryable    *bool  `json:"retryable"`
		Fatal        *bool  `json:"fatal"`
		RetryAfterMs *int64 `json:"retryAfterMs,omitempty"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return fmt.Errorf("decode error payload: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return err
	}
	if payload.Code == "" || payload.Message == "" || payload.Retryable == nil || payload.Fatal == nil {
		return errors.New("error payload requires code, message, retryable and fatal")
	}
	return nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("unexpected trailing JSON value")
		}
		return fmt.Errorf("decode trailing JSON: %w", err)
	}
	return nil
}

func TurnKey(turnID string) uint64 {
	hash := sha256.Sum256([]byte(turnID))
	return binary.BigEndian.Uint64(hash[:8])
}

type FrameType uint8

const (
	FrameTypeInputPCM     FrameType = 1
	FrameTypeAssistantMP3 FrameType = 2
)

const (
	FlagStart uint8 = 1 << iota
	FlagEnd
)

const allowedBinaryFlags = FlagStart | FlagEnd

type BinaryFrame struct {
	FrameType  FrameType
	Flags      uint8
	Generation uint32
	TurnKey    uint64
	AudioSeq   uint32
	SegmentSeq uint32
	Payload    []byte
}

func EncodeBinaryFrame(frame BinaryFrame) ([]byte, error) {
	if err := validateBinaryFrame(frame); err != nil {
		return nil, err
	}
	encoded := make([]byte, BinaryHeaderLength+len(frame.Payload))
	copy(encoded[0:4], "XZV1")
	encoded[4] = 1
	encoded[5] = byte(frame.FrameType)
	encoded[6] = frame.Flags
	encoded[7] = BinaryHeaderLength
	binary.BigEndian.PutUint32(encoded[8:12], frame.Generation)
	binary.BigEndian.PutUint64(encoded[12:20], frame.TurnKey)
	binary.BigEndian.PutUint32(encoded[20:24], frame.AudioSeq)
	binary.BigEndian.PutUint32(encoded[24:28], frame.SegmentSeq)
	binary.BigEndian.PutUint32(encoded[28:32], uint32(len(frame.Payload)))
	copy(encoded[BinaryHeaderLength:], frame.Payload)
	return encoded, nil
}

func DecodeBinaryFrame(data []byte) (BinaryFrame, error) {
	if len(data) < BinaryHeaderLength {
		return BinaryFrame{}, invalidBinaryFrame("header is shorter than 32 bytes")
	}
	if string(data[0:4]) != "XZV1" {
		return BinaryFrame{}, invalidBinaryFrame("magic must be XZV1")
	}
	if data[4] != 1 {
		return BinaryFrame{}, invalidBinaryFrame("version must be 1")
	}
	if data[7] != BinaryHeaderLength {
		return BinaryFrame{}, invalidBinaryFrame("headerLength must be 32")
	}
	payloadLength := binary.BigEndian.Uint32(data[28:32])
	if uint64(payloadLength) != uint64(len(data)-BinaryHeaderLength) {
		return BinaryFrame{}, invalidBinaryFrame("payloadLength does not match remaining bytes")
	}
	frame := BinaryFrame{
		FrameType:  FrameType(data[5]),
		Flags:      data[6],
		Generation: binary.BigEndian.Uint32(data[8:12]),
		TurnKey:    binary.BigEndian.Uint64(data[12:20]),
		AudioSeq:   binary.BigEndian.Uint32(data[20:24]),
		SegmentSeq: binary.BigEndian.Uint32(data[24:28]),
		Payload:    bytes.Clone(data[BinaryHeaderLength:]),
	}
	if err := validateBinaryFrame(frame); err != nil {
		return BinaryFrame{}, err
	}
	return frame, nil
}

func validateBinaryFrame(frame BinaryFrame) error {
	if int64(len(frame.Payload)) > MaxBinaryPayloadLength {
		return invalidBinaryFrame("payload exceeds uint32 length")
	}
	if frame.Flags&^allowedBinaryFlags != 0 {
		return invalidBinaryFrame("reserved flags must be zero")
	}
	switch frame.FrameType {
	case FrameTypeInputPCM:
		if frame.Flags&FlagEnd != 0 {
			return invalidBinaryFrame("input_pcm frame cannot set end flag")
		}
		if (frame.AudioSeq == 0) != (frame.Flags&FlagStart != 0) {
			return invalidBinaryFrame("input_pcm start flag must be set exactly when audioSeq is zero")
		}
		if frame.SegmentSeq != 0 {
			return invalidBinaryFrame("input_pcm segmentSeq must be zero")
		}
	case FrameTypeAssistantMP3:
		if frame.Flags != FlagStart|FlagEnd {
			return invalidBinaryFrame("assistant_mp3 frame must set start and end flags")
		}
	default:
		return invalidBinaryFrame("unknown frameType")
	}
	return nil
}

func invalidBinaryFrame(message string) error {
	return fmt.Errorf("%w: %s", ErrInvalidBinaryFrame, message)
}

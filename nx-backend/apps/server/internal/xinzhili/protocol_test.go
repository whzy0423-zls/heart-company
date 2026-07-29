package xinzhili

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

func uint64ptr(v uint64) *uint64 { return &v }
func stringptr(v string) *string { return &v }

func validEnvelope(eventType EventType) Envelope {
	return Envelope{
		ProtocolVersion: ProtocolVersion,
		Type:            eventType,
		SessionID:       stringptr("session-1"),
		Generation:      3,
		SessionSeq:      uint64ptr(0),
		ConfigVersion:   7,
		TimestampMs:     1720000000123,
		Payload:         json.RawMessage(`{}`),
	}
}

func TestProtocolEnvelopeJSONUsesVersionedPublicFields(t *testing.T) {
	envelope := validEnvelope(EventSessionPing)
	envelope.TurnID = nil
	envelope.TurnSeq = nil

	encoded, err := EncodeEnvelope(envelope, DirectionClient, true)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"protocolVersion":"xinzhili.voice.v1","type":"session.ping","sessionId":"session-1","generation":3,"turnId":null,"sessionSeq":0,"turnSeq":null,"configVersion":7,"timestampMs":1720000000123,"payload":{}}`
	if string(encoded) != want {
		t.Fatalf("encoded=%s\nwant=%s", encoded, want)
	}

	decoded, err := DecodeEnvelope(encoded, DirectionClient, true)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Type != EventSessionPing || decoded.SessionID == nil || *decoded.SessionID != "session-1" {
		t.Fatalf("decoded=%+v", decoded)
	}
}

func TestProtocolEnvelopeRejectsUnknownOrWrongDirectionEvent(t *testing.T) {
	tests := []struct {
		name      string
		eventType EventType
		direction Direction
	}{
		{"unknown", EventType("session.mystery"), DirectionClient},
		{"server event from client", EventSessionReady, DirectionClient},
		{"client event from server", EventSessionStart, DirectionServer},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envelope := validEnvelope(tt.eventType)
			if err := ValidateEnvelope(envelope, tt.direction, true); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestProtocolEventNamesAreVersionedContract(t *testing.T) {
	want := map[EventType]string{
		EventSessionStart:         "session.start",
		EventSessionStop:          "session.stop",
		EventSessionPing:          "session.ping",
		EventTurnStart:            "turn.start",
		EventTurnCancel:           "turn.cancel",
		EventModeChange:           "mode.change",
		EventPlaybackInterrupt:    "playback.interrupt",
		EventAssistantPlaybackAck: "assistant.playback_ack",
		EventSessionReady:         "session.ready",
		EventSessionPong:          "session.pong",
		EventConfigChanged:        "session.config_changed",
		EventModeChanged:          "session.mode_changed",
		EventASRActivity:          "asr.activity",
		EventTurnProcessing:       "turn.processing",
		EventTurnCancelled:        "turn.cancelled",
		EventAssistantAudioStart:  "assistant.audio_start",
		EventAssistantAudioEnd:    "assistant.audio_end",
		EventAssistantDone:        "assistant.done",
		EventError:                "error",
	}
	for eventType, expected := range want {
		if string(eventType) != expected {
			t.Errorf("event=%q want=%q", eventType, expected)
		}
	}
}

func TestEnvelopeSessionIDLifecycle(t *testing.T) {
	tests := []struct {
		name         string
		eventType    EventType
		direction    Direction
		sessionReady bool
		sessionID    *string
		wantError    bool
	}{
		{"session start before ready permits null", EventSessionStart, DirectionClient, false, nil, false},
		{"session start before ready rejects id", EventSessionStart, DirectionClient, false, stringptr("session-1"), true},
		{"session start after ready requires id", EventSessionStart, DirectionClient, true, nil, true},
		{"session start after ready rejects restart", EventSessionStart, DirectionClient, true, stringptr("session-1"), true},
		{"session ready requires id", EventSessionReady, DirectionServer, false, nil, true},
		{"post-ready event requires id", EventSessionPing, DirectionClient, true, nil, true},
		{"post-ready event accepts id", EventSessionPing, DirectionClient, true, stringptr("session-1"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envelope := validEnvelope(tt.eventType)
			envelope.SessionID = tt.sessionID
			err := ValidateEnvelope(envelope, tt.direction, tt.sessionReady)
			if (err != nil) != tt.wantError {
				t.Fatalf("err=%v wantError=%v", err, tt.wantError)
			}
		})
	}
}

func TestDecodeEnvelopeKeepsSafetyCriticalFieldsRequired(t *testing.T) {
	base := []byte(`{"protocolVersion":"xinzhili.voice.v1","type":"session.ping","sessionId":"session-1","generation":0,"turnId":null,"sessionSeq":0,"turnSeq":null,"configVersion":0,"timestampMs":1,"payload":{}}`)
	required := []string{"protocolVersion", "type", "timestampMs"}
	for _, field := range required {
		for _, variant := range []string{"omitted", "null"} {
			t.Run(field+"_"+variant, func(t *testing.T) {
				var object map[string]json.RawMessage
				if err := json.Unmarshal(base, &object); err != nil {
					t.Fatal(err)
				}
				if variant == "omitted" {
					delete(object, field)
				} else {
					object[field] = json.RawMessage("null")
				}
				encoded, err := json.Marshal(object)
				if err != nil {
					t.Fatal(err)
				}
				_, err = DecodeEnvelope(encoded, DirectionClient, true)
				if err == nil {
					t.Fatal("expected decode error")
				}
				var protocolErr *ProtocolError
				if !errors.As(err, &protocolErr) {
					t.Fatalf("err=%T %v", err, err)
				}
				if protocolErr.Code != ProtocolErrorInvalidEnvelope {
					t.Fatalf("code=%q want=%q", protocolErr.Code, ProtocolErrorInvalidEnvelope)
				}
			})
		}
	}
}

func TestDecodeEnvelopeAcceptsLegacyV1DefaultsAndUnknownOptionalFields(t *testing.T) {
	data := []byte(`{"protocolVersion":"xinzhili.voice.v1","type":"session.ping","sessionId":"session-1","turnId":null,"turnSeq":null,"timestampMs":1,"futureOptional":"ignored"}`)
	decoded, err := DecodeEnvelope(data, DirectionClient, true)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Generation != 0 || decoded.SessionSeq == nil || *decoded.SessionSeq != 0 || decoded.ConfigVersion != 0 {
		t.Fatalf("legacy defaults not applied: %+v", decoded)
	}
	if string(decoded.Payload) != "{}" {
		t.Fatalf("payload=%s want={}", decoded.Payload)
	}
}

func TestDecodeEnvelopeKeepsSafetyChecksForLegacyV1(t *testing.T) {
	tests := []struct {
		name string
		data string
		code string
	}{
		{"wrong version", `{"protocolVersion":"xinzhili.voice.v2","type":"session.ping","sessionId":"session-1","timestampMs":1}`, ProtocolErrorUnsupportedVersion},
		{"wrong direction", `{"protocolVersion":"xinzhili.voice.v1","type":"session.ready","sessionId":"session-1","timestampMs":1}`, ProtocolErrorInvalidEventDirection},
		{"array payload", `{"protocolVersion":"xinzhili.voice.v1","type":"session.ping","sessionId":"session-1","timestampMs":1,"payload":[]}`, ProtocolErrorInvalidPayload},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeEnvelope([]byte(tt.data), DirectionClient, true)
			var protocolErr *ProtocolError
			if !errors.As(err, &protocolErr) || protocolErr.Code != tt.code {
				t.Fatalf("err=%T %v wantCode=%q", err, err, tt.code)
			}
		})
	}
}

func TestDecodeEnvelopeAcceptsZeroRequiredNumbers(t *testing.T) {
	data := []byte(`{"protocolVersion":"xinzhili.voice.v1","type":"session.ping","sessionId":"session-1","generation":0,"turnId":null,"sessionSeq":0,"turnSeq":null,"configVersion":0,"timestampMs":1,"payload":{}}`)
	decoded, err := DecodeEnvelope(data, DirectionClient, true)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Generation != 0 || decoded.SessionSeq == nil || *decoded.SessionSeq != 0 || decoded.ConfigVersion != 0 {
		t.Fatalf("decoded=%+v", decoded)
	}
}

func TestEnvelopeTimestampMustBePositive(t *testing.T) {
	for _, timestampMs := range []int64{0, -1} {
		t.Run(fmt.Sprintf("timestamp_%d", timestampMs), func(t *testing.T) {
			envelope := validEnvelope(EventSessionPing)
			envelope.TimestampMs = timestampMs
			err := ValidateEnvelope(envelope, DirectionClient, true)
			var protocolErr *ProtocolError
			if !errors.As(err, &protocolErr) || protocolErr.Code != ProtocolErrorInvalidEnvelope {
				t.Fatalf("err=%T %v", err, err)
			}
		})
	}
}

func TestProtocolErrorDiagnosticIdentifiesFailingField(t *testing.T) {
	data := []byte(`{"protocolVersion":"xinzhili.voice.v1","type":"session.ping","sessionId":"session-1","timestampMs":0}`)
	_, err := DecodeEnvelope(data, DirectionClient, true)
	var protocolErr *ProtocolError
	if !errors.As(err, &protocolErr) {
		t.Fatalf("err=%T %v", err, err)
	}
	if protocolErr.Code != ProtocolErrorInvalidEnvelope || protocolErr.Field != "timestampMs" {
		t.Fatalf("protocolErr=%+v", protocolErr)
	}
	if protocolErr.Reason == "" {
		t.Fatal("diagnostic reason is empty")
	}
}

func TestEnvelopePayloadMustBeJSONObject(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{"null", "null"},
		{"number", "1"},
		{"string", `"text"`},
		{"array", `[]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envelope := validEnvelope(EventSessionPing)
			envelope.Payload = json.RawMessage(tt.payload)
			err := ValidateEnvelope(envelope, DirectionClient, true)
			var protocolErr *ProtocolError
			if !errors.As(err, &protocolErr) || protocolErr.Code != ProtocolErrorInvalidPayload {
				t.Fatalf("err=%T %v", err, err)
			}
		})
	}
}

func TestEnvelopeRequiresSessionOrTurnFieldsByEventLevel(t *testing.T) {
	tests := []struct {
		name      string
		eventType EventType
		direction Direction
		turnID    *string
		turnSeq   *uint64
		wantError bool
	}{
		{"session event null turn fields", EventModeChange, DirectionClient, nil, nil, false},
		{"session event rejects turn id", EventModeChange, DirectionClient, stringptr("turn-1"), nil, true},
		{"session event rejects turn seq", EventError, DirectionServer, nil, uint64ptr(0), true},
		{"turn event requires turn id", EventASRActivity, DirectionServer, nil, uint64ptr(0), true},
		{"turn event requires turn seq", EventPlaybackInterrupt, DirectionClient, stringptr("turn-1"), nil, true},
		{"turn event accepts both", EventAssistantPlaybackAck, DirectionClient, stringptr("turn-1"), uint64ptr(0), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envelope := validEnvelope(tt.eventType)
			envelope.TurnID = tt.turnID
			envelope.TurnSeq = tt.turnSeq
			err := ValidateEnvelope(envelope, tt.direction, true)
			if (err != nil) != tt.wantError {
				t.Fatalf("err=%v wantError=%v", err, tt.wantError)
			}
		})
	}
}

func TestEnvelopeRequiresProtocolAndSessionSequence(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Envelope)
	}{
		{"protocol version", func(e *Envelope) { e.ProtocolVersion = "v2" }},
		{"session sequence", func(e *Envelope) { e.SessionSeq = nil }},
		{"payload", func(e *Envelope) { e.Payload = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envelope := validEnvelope(EventSessionPing)
			tt.mutate(&envelope)
			if err := ValidateEnvelope(envelope, DirectionClient, true); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestEnvelopeTurnStartValidatesTurnKey(t *testing.T) {
	turnID := "turn-123"
	tests := []struct {
		name      string
		payload   string
		wantError bool
	}{
		{"matching", `{"turnKey":6919734513873532354}`, false},
		{"mismatch", `{"turnKey":1}`, true},
		{"missing", `{}`, true},
		{"unknown", `{"turnKey":6919734513873532354,"extra":true}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envelope := validEnvelope(EventTurnStart)
			envelope.TurnID = &turnID
			envelope.TurnSeq = uint64ptr(0)
			envelope.Payload = json.RawMessage(tt.payload)
			err := ValidateEnvelope(envelope, DirectionClient, true)
			if (err != nil) != tt.wantError {
				t.Fatalf("err=%v wantError=%v", err, tt.wantError)
			}
		})
	}
}

func TestProtocolErrorCodesAreStableAndUnwrap(t *testing.T) {
	tests := []struct {
		name      string
		data      []byte
		direction Direction
		wantCode  string
	}{
		{"invalid envelope", []byte(`{"protocolVersion":`), DirectionClient, ProtocolErrorInvalidEnvelope},
		{"unsupported version", []byte(`{"protocolVersion":"v2","type":"session.ping","sessionId":"s","generation":0,"turnId":null,"sessionSeq":0,"turnSeq":null,"configVersion":0,"timestampMs":1,"payload":{}}`), DirectionClient, ProtocolErrorUnsupportedVersion},
		{"invalid direction", []byte(`{"protocolVersion":"xinzhili.voice.v1","type":"session.ready","sessionId":"s","generation":0,"turnId":null,"sessionSeq":0,"turnSeq":null,"configVersion":0,"timestampMs":1,"payload":{}}`), DirectionClient, ProtocolErrorInvalidEventDirection},
		{"invalid payload", []byte(`{"protocolVersion":"xinzhili.voice.v1","type":"session.ping","sessionId":"s","generation":0,"turnId":null,"sessionSeq":0,"turnSeq":null,"configVersion":0,"timestampMs":1,"payload":[]}`), DirectionClient, ProtocolErrorInvalidPayload},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeEnvelope(tt.data, tt.direction, true)
			var protocolErr *ProtocolError
			if !errors.As(err, &protocolErr) || protocolErr.Code != tt.wantCode || protocolErr.Unwrap() == nil {
				t.Fatalf("err=%T %v", err, err)
			}
		})
	}
	cause := errors.New("cause")
	wrapped := &ProtocolError{Code: ProtocolErrorInvalidEnvelope, Err: cause}
	if !errors.Is(wrapped, cause) {
		t.Fatal("ProtocolError must unwrap for errors.Is")
	}
}

func TestProtocolErrorPayloadShape(t *testing.T) {
	tests := []struct {
		name      string
		payload   string
		wantError bool
	}{
		{"required fields", `{"code":"control_sequence_gap","message":"序号存在缺口","retryable":true,"fatal":false}`, false},
		{"optional retry", `{"code":"busy","message":"稍后重试","retryable":true,"fatal":false,"retryAfterMs":250}`, false},
		{"missing code", `{"message":"x","retryable":false,"fatal":true}`, true},
		{"unknown field", `{"code":"x","message":"x","retryable":false,"fatal":true,"details":"secret"}`, true},
		{"immediate retry", `{"code":"x","message":"x","retryable":true,"fatal":false,"retryAfterMs":0}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envelope := validEnvelope(EventError)
			envelope.Payload = json.RawMessage(tt.payload)
			err := ValidateEnvelope(envelope, DirectionServer, true)
			if (err != nil) != tt.wantError {
				t.Fatalf("err=%v wantError=%v", err, tt.wantError)
			}
		})
	}
}

func TestTurnKeyFixedVector(t *testing.T) {
	const want uint64 = 0xf23091852608511f
	if got := TurnKey("turn-fixed-vector"); got != want {
		t.Fatalf("TurnKey=%#x want=%#x", got, want)
	}
}

func TestBinaryHeaderFixedVector(t *testing.T) {
	frame := BinaryFrame{
		FrameType:  FrameTypeAssistantMP3,
		Flags:      FlagStart | FlagEnd,
		Generation: 0x01020304,
		TurnKey:    0x1112131415161718,
		AudioSeq:   0x21222324,
		SegmentSeq: 0x31323334,
		Payload:    []byte{0xaa, 0xbb, 0xcc},
	}
	want := []byte{
		0x58, 0x5a, 0x56, 0x31,
		0x01, 0x02, 0x03, 0x20,
		0x01, 0x02, 0x03, 0x04,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18,
		0x21, 0x22, 0x23, 0x24,
		0x31, 0x32, 0x33, 0x34,
		0x00, 0x00, 0x00, 0x03,
		0xaa, 0xbb, 0xcc,
	}
	encoded, err := EncodeBinaryFrame(frame)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, want) {
		t.Fatalf("encoded=% x\nwant=% x", encoded, want)
	}
	decoded, err := DecodeBinaryFrame(want)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.FrameType != frame.FrameType || decoded.Flags != frame.Flags || decoded.Generation != frame.Generation || decoded.TurnKey != frame.TurnKey || decoded.AudioSeq != frame.AudioSeq || decoded.SegmentSeq != frame.SegmentSeq || !bytes.Equal(decoded.Payload, frame.Payload) {
		t.Fatalf("decoded=%+v want=%+v", decoded, frame)
	}
}

func TestBinaryHeaderRejectsMalformedFields(t *testing.T) {
	valid := []byte{
		0x58, 0x5a, 0x56, 0x31, 0x01, 0x01, 0x01, 0x20,
		0x00, 0x00, 0x00, 0x01, 0x60, 0x07, 0xd5, 0xe0,
		0xc7, 0x71, 0x35, 0xc2, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02,
		0x10, 0x20,
	}
	tests := []struct {
		name   string
		mutate func([]byte) []byte
	}{
		{"short", func(v []byte) []byte { return v[:31] }},
		{"magic", func(v []byte) []byte { v[0] = 'Y'; return v }},
		{"version", func(v []byte) []byte { v[4] = 2; return v }},
		{"unknown frame type", func(v []byte) []byte { v[5] = 3; return v }},
		{"reserved flags", func(v []byte) []byte { v[6] = 0x04; return v }},
		{"header length", func(v []byte) []byte { v[7] = 31; return v }},
		{"payload shorter", func(v []byte) []byte { v[31] = 3; return v }},
		{"payload longer", func(v []byte) []byte { return v[:33] }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := tt.mutate(bytes.Clone(valid))
			if _, err := DecodeBinaryFrame(candidate); err == nil {
				t.Fatal("expected decode error")
			}
		})
	}
}

func TestBinaryHeaderValidatesFrameSpecificFlagsAndSegments(t *testing.T) {
	tests := []struct {
		name      string
		frame     BinaryFrame
		wantError bool
	}{
		{"input zero missing start", BinaryFrame{FrameType: FrameTypeInputPCM, Payload: []byte{1}}, true},
		{"input start", BinaryFrame{FrameType: FrameTypeInputPCM, Flags: FlagStart, Payload: []byte{1}}, false},
		{"input late start", BinaryFrame{FrameType: FrameTypeInputPCM, Flags: FlagStart, AudioSeq: 1, Payload: []byte{1}}, true},
		{"input continuation", BinaryFrame{FrameType: FrameTypeInputPCM, AudioSeq: 1, Payload: []byte{1}}, false},
		{"input end forbidden", BinaryFrame{FrameType: FrameTypeInputPCM, Flags: FlagEnd, Payload: []byte{1}}, true},
		{"input segment nonzero", BinaryFrame{FrameType: FrameTypeInputPCM, SegmentSeq: 1, Payload: []byte{1}}, true},
		{"tts complete", BinaryFrame{FrameType: FrameTypeAssistantMP3, Flags: FlagStart | FlagEnd, Payload: []byte{1}}, false},
		{"tts missing start", BinaryFrame{FrameType: FrameTypeAssistantMP3, Flags: FlagEnd, Payload: []byte{1}}, true},
		{"tts missing end", BinaryFrame{FrameType: FrameTypeAssistantMP3, Flags: FlagStart, Payload: []byte{1}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := EncodeBinaryFrame(tt.frame)
			if (err != nil) != tt.wantError {
				t.Fatalf("err=%v wantError=%v", err, tt.wantError)
			}
		})
	}
}

func TestBinaryWirePayloadLengthUsesUint32Contract(t *testing.T) {
	if MaxBinaryPayloadLength != int64(^uint32(0)) {
		t.Fatalf("MaxBinaryPayloadLength=%d", MaxBinaryPayloadLength)
	}
}

func TestDecodeEnvelopeAcceptsPublishedV1DefaultsAndUnknownFields(t *testing.T) {
	for _, data := range [][]byte{
		[]byte(`{"protocolVersion":"xinzhili.voice.v1","type":"session.start","sessionId":null,"timestampMs":1,"payload":{},"clientExtra":"kept-by-client"}`),
		[]byte(`{"protocolVersion":"xinzhili.voice.v1","type":"session.start","sessionId":null,"timestampMs":1}`),
	} {
		got, err := DecodeEnvelope(data, DirectionClient, false)
		if err != nil {
			t.Fatalf("DecodeEnvelope(%s): %v", data, err)
		}
		if got.Generation != 0 || got.SessionSeq == nil || *got.SessionSeq != 0 || got.ConfigVersion != 0 {
			t.Fatalf("defaults = generation %d sessionSeq %v configVersion %d", got.Generation, got.SessionSeq, got.ConfigVersion)
		}
		if string(got.Payload) != "{}" {
			t.Fatalf("payload = %s", got.Payload)
		}
	}
}

func TestDecodeEnvelopeRejectsExplicitNullPayload(t *testing.T) {
	data := []byte(`{"protocolVersion":"xinzhili.voice.v1","type":"session.start","sessionId":null,"timestampMs":1,"payload":null}`)
	_, err := DecodeEnvelope(data, DirectionClient, false)
	var protocolErr *ProtocolError
	if !errors.As(err, &protocolErr) {
		t.Fatalf("err = %T %v", err, err)
	}
	if protocolErr.Code != ProtocolErrorInvalidPayload {
		t.Fatalf("code = %q, want %q", protocolErr.Code, ProtocolErrorInvalidPayload)
	}
}

func TestDecodeEnvelopeRejectsExplicitNullDefaultableFields(t *testing.T) {
	base := []byte(`{"protocolVersion":"xinzhili.voice.v1","type":"session.start","sessionId":null,"generation":0,"sessionSeq":0,"configVersion":0,"timestampMs":1,"payload":{}}`)
	for _, field := range []string{"generation", "sessionSeq", "configVersion"} {
		t.Run(field, func(t *testing.T) {
			var object map[string]json.RawMessage
			if err := json.Unmarshal(base, &object); err != nil {
				t.Fatal(err)
			}
			object[field] = json.RawMessage("null")
			data, err := json.Marshal(object)
			if err != nil {
				t.Fatal(err)
			}
			_, err = DecodeEnvelope(data, DirectionClient, false)
			var protocolErr *ProtocolError
			if !errors.As(err, &protocolErr) {
				t.Fatalf("err = %T %v", err, err)
			}
			if protocolErr.Code != ProtocolErrorInvalidEnvelope {
				t.Fatalf("code = %q, want %q", protocolErr.Code, ProtocolErrorInvalidEnvelope)
			}
		})
	}
}

func FuzzDecodeEnvelope(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte(`{"protocolVersion":"xinzhili.voice.v1","type":"session.ping","sessionId":"s","generation":0,"turnId":null,"sessionSeq":0,"turnSeq":null,"configVersion":0,"timestampMs":1,"payload":{}}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		envelope, err := DecodeEnvelope(data, DirectionClient, true)
		if err != nil {
			return
		}
		encoded, err := EncodeEnvelope(envelope, DirectionClient, true)
		if err != nil {
			t.Fatalf("decoded envelope cannot encode: %v", err)
		}
		if _, err := DecodeEnvelope(encoded, DirectionClient, true); err != nil {
			t.Fatalf("encoded envelope cannot decode: %v", err)
		}
	})
}

func FuzzDecodeBinaryFrame(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte("XZV1"))
	f.Add([]byte{
		0x58, 0x5a, 0x56, 0x31, 0x01, 0x01, 0x01, 0x20,
		0, 0, 0, 1, 0x60, 0x07, 0xd5, 0xe0, 0xc7, 0x71, 0x35, 0xc2,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0x01,
	})
	f.Fuzz(func(t *testing.T, data []byte) {
		frame, err := DecodeBinaryFrame(data)
		if err != nil {
			return
		}
		encoded, err := EncodeBinaryFrame(frame)
		if err != nil {
			t.Fatalf("decoded frame cannot encode: %v", err)
		}
		if !bytes.Equal(encoded, data) {
			t.Fatalf("round trip mismatch")
		}
	})
}

func TestProtocolErrorsAreClassifiable(t *testing.T) {
	_, err := DecodeBinaryFrame([]byte("short"))
	if !errors.Is(err, ErrInvalidBinaryFrame) {
		t.Fatalf("err=%v", err)
	}
}

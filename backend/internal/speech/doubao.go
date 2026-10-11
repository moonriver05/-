package speech

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const MaxPCMBytes = 16000 * 2 * 60
const MaxFrameBytes = 6400 // 200 ms of mono 16 kHz PCM16.
const ResourceID = "volc.seedasr.sauc.duration"

type Client struct{ key, endpoint string }

func FromEnv(provider string) *Client {
	key := ""
	if strings.EqualFold(provider, "doubao") {
		key = strings.TrimSpace(os.Getenv("DOUBAO_SPEECH_API_KEY"))
	}
	return &Client{key: key, endpoint: "wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_nostream"}
}
func (c *Client) Configured() bool { return c != nil && c.key != "" }
func (c *Client) Open(ctx context.Context, uid string) (*websocket.Conn, error) {
	if !c.Configured() {
		return nil, errors.New("语音识别尚未配置")
	}
	headers := http.Header{"X-Api-Key": {c.key}, "X-Api-Resource-Id": {ResourceID}, "X-Api-Request-Id": {uuid.NewString()}}
	dialer := websocket.Dialer{HandshakeTimeout: 8 * time.Second}
	conn, response, err := dialer.DialContext(ctx, c.endpoint, headers)
	if response != nil && response.Body != nil {
		response.Body.Close()
	}
	if err != nil {
		return nil, errors.New("无法连接豆包语音，请检查服务开通状态和语音 API Key")
	}
	conn.SetReadLimit(1 << 20)
	deadline, _ := ctx.Deadline()
	conn.SetReadDeadline(deadline)
	conn.SetWriteDeadline(deadline)
	payload, _ := json.Marshal(map[string]any{
		"user":    map[string]string{"uid": uid},
		"audio":   map[string]any{"format": "pcm", "codec": "raw", "rate": 16000, "bits": 16, "channel": 1},
		"request": map[string]any{"model_name": "bigmodel", "enable_itn": true, "enable_punc": true, "result_type": "full"},
	})
	if err = conn.WriteMessage(websocket.BinaryMessage, Frame(1, false, payload)); err != nil {
		conn.Close()
		return nil, errors.New("语音连接中断，请重试")
	}
	return conn, nil
}

// Frame uses the documented four-byte header, big-endian size, and raw payload.
func Frame(kind byte, final bool, payload []byte) []byte {
	flags := byte(0)
	if final {
		flags = 2
	}
	serialization := byte(0)
	if kind == 1 {
		serialization = 0x10
	}
	frame := make([]byte, 8+len(payload))
	copy(frame, []byte{0x11, kind<<4 | flags, serialization, 0})
	binary.BigEndian.PutUint32(frame[4:8], uint32(len(payload)))
	copy(frame[8:], payload)
	return frame
}

// Parse accepts sequenced and unsequenced replies, including gzip compression.
func Parse(frame []byte) (text string, final bool, err error) {
	invalid := errors.New("语音服务返回了无效数据")
	if len(frame) < 8 || frame[0]>>4 != 1 {
		return "", false, invalid
	}
	offset := int(frame[0]&15) * 4
	if offset < 4 || offset+4 > len(frame) {
		return "", false, invalid
	}
	kind, flags := frame[1]>>4, frame[1]&15
	final = flags&2 != 0
	if flags&1 != 0 {
		offset += 4
	}
	var code uint32
	if kind == 15 {
		if offset+4 > len(frame) {
			return "", false, invalid
		}
		code = binary.BigEndian.Uint32(frame[offset : offset+4])
		offset += 4
	} else if kind != 9 {
		return "", false, invalid
	}
	if offset+4 > len(frame) {
		return "", false, invalid
	}
	size := int(binary.BigEndian.Uint32(frame[offset : offset+4]))
	offset += 4
	if size > 1<<20 || size != len(frame)-offset {
		return "", false, invalid
	}
	payload := frame[offset:]
	switch frame[2] & 15 {
	case 1:
		reader, e := gzip.NewReader(bytes.NewReader(payload))
		if e != nil {
			return "", false, invalid
		}
		defer reader.Close()
		payload, e = io.ReadAll(io.LimitReader(reader, (1<<20)+1))
		if e != nil || len(payload) > 1<<20 {
			return "", false, invalid
		}
	case 0:
	default:
		return "", false, invalid
	}
	if kind == 15 {
		return "", false, fmt.Errorf("豆包语音识别失败（代码 %d），请检查额度或稍后重试", code)
	}
	var result struct {
		Code   int             `json:"code"`
		Result json.RawMessage `json:"result"`
	}
	if json.Unmarshal(payload, &result) != nil {
		return "", false, invalid
	}
	if result.Code != 0 && result.Code != 20000000 {
		return "", false, fmt.Errorf("豆包语音识别失败（代码 %d）", result.Code)
	}
	var single struct {
		Text string `json:"text"`
	}
	if len(result.Result) == 0 || string(result.Result) == "null" {
		return "", final, nil
	}
	if json.Unmarshal(result.Result, &single) == nil {
		return single.Text, final, nil
	}
	var many []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(result.Result, &many) != nil {
		return "", false, invalid
	}
	var parts []string
	for _, item := range many {
		parts = append(parts, item.Text)
	}
	return strings.Join(parts, ""), final, nil
}

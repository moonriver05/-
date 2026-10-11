package speech

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/json"
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	for _, payload := range []string{`{"result":{"text":"微积分"}}`, `{"result":[{"text":"微"},{"text":"积分"}]}`} {
		var buf bytes.Buffer
		writer := gzip.NewWriter(&buf)
		writer.Write([]byte(payload))
		writer.Close()
		frame := Frame(9, true, buf.Bytes())
		frame[1] = 0x93
		frame[2] = 0x11
		frame = append(frame[:4], append([]byte{255, 255, 255, 255}, frame[4:]...)...)
		text, final, err := Parse(frame)
		if err != nil || !final || text != "微积分" {
			t.Fatalf("%q %v %v", text, final, err)
		}
	}
	for _, frame := range [][]byte{nil, {0x11, 0x93, 0x11, 0}, Frame(9, false, []byte(`{`)), Frame(9, false, []byte(`{"code":45000001}`))} {
		if _, _, err := Parse(frame); err == nil {
			t.Fatal("invalid reply accepted")
		}
	}
	frame := Frame(15, false, []byte("secret vendor error"))
	frame = append(frame[:4], append([]byte{0, 0, 0, 42}, frame[4:]...)...)
	_, _, err := Parse(frame)
	if err == nil || strings.Contains(err.Error(), "secret") || !strings.Contains(err.Error(), "42") {
		t.Fatal(err)
	}
}

func TestOpenAndAudioProtocol(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "test-key" || r.Header.Get("X-Api-Resource-Id") != ResourceID || r.Header.Get("X-Api-Request-Id") == "" {
			t.Error("missing authentication")
		}
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		_, frame, err := conn.ReadMessage()
		if err != nil {
			t.Error(err)
			return
		}
		if len(frame) < 8 || frame[1] != 0x10 || int(binary.BigEndian.Uint32(frame[4:8])) != len(frame)-8 {
			t.Error("invalid config frame")
			return
		}
		var cfg struct {
			Audio struct {
				Rate   int
				Format string
			}
			Request struct {
				ModelName string `json:"model_name"`
			}
		}
		if json.Unmarshal(frame[8:], &cfg) != nil || cfg.Audio.Rate != 16000 || cfg.Audio.Format != "pcm" || cfg.Request.ModelName != "bigmodel" {
			t.Error("incorrect audio metadata")
		}
		_, frame, err = conn.ReadMessage()
		if err != nil {
			t.Error(err)
			return
		}
		if frame[1] != 0x22 {
			t.Error("missing final audio flag")
		}
		conn.WriteMessage(websocket.BinaryMessage, Frame(9, true, []byte(`{"result":{"text":"你好"}}`)))
	}))
	defer server.Close()
	client := &Client{key: "test-key", endpoint: "ws" + strings.TrimPrefix(server.URL, "http")}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()
	conn, err := client.Open(ctx, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err = conn.WriteMessage(websocket.BinaryMessage, Frame(2, true, []byte{0, 0})); err != nil {
		t.Fatal(err)
	}
	_, frame, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	text, final, err := Parse(frame)
	if err != nil || !final || text != "你好" {
		t.Fatalf("%q %v %v", text, final, err)
	}
}

func TestConfiguredRequiresProviderAndKey(t *testing.T) {
	t.Setenv("DOUBAO_SPEECH_API_KEY", "test-key")
	if FromEnv("mock").Configured() || !FromEnv("doubao").Configured() {
		t.Fatal("provider ignored")
	}
	t.Setenv("DOUBAO_SPEECH_API_KEY", "")
	if FromEnv("doubao").Configured() {
		t.Fatal("missing key accepted")
	}
}

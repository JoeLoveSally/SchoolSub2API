package hkust

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/websocket"
)

func TestProbeModelAvailable(t *testing.T) {
	server := newProbeServer(t, 0, "JOEJOEPROXY_OK")
	defer server.Close()

	client := NewClient(probeTestConfig(server.URL))
	client.heartbeatInterval = time.Hour
	result := client.ProbeModel(context.Background(), "Kimi-K3", time.Second)
	if result.Status != "available" {
		t.Fatalf("status = %q, message = %q, want available", result.Status, result.Message)
	}
	if result.Model != "Kimi-K3" {
		t.Fatalf("model = %q, want Kimi-K3", result.Model)
	}
}

func TestProbeModelRejectsUnexpectedResponse(t *testing.T) {
	server := newProbeServer(t, 0, "LiteLLM upstream error")
	defer server.Close()

	client := NewClient(probeTestConfig(server.URL))
	client.heartbeatInterval = time.Hour
	result := client.ProbeModel(context.Background(), "GLM-5.2", time.Second)
	if result.Status != "failed" {
		t.Fatalf("status = %q, message = %q, want failed", result.Status, result.Message)
	}
}

func TestProbeModelTimeout(t *testing.T) {
	server := newProbeServer(t, 250*time.Millisecond, "JOEJOEPROXY_OK")
	defer server.Close()

	client := NewClient(probeTestConfig(server.URL))
	client.heartbeatInterval = time.Hour
	result := client.ProbeModel(context.Background(), "DeepSeek-V4-Flash-conv", 50*time.Millisecond)
	if result.Status != "timeout" {
		t.Fatalf("status = %q, message = %q, want timeout", result.Status, result.Message)
	}
}

func TestSupportedProbeModelIDsReturnsCopy(t *testing.T) {
	models := SupportedProbeModelIDs()
	if len(models) != 4 {
		t.Fatalf("len(models) = %d, want 4", len(models))
	}
	models[0] = "changed"
	if SupportedProbeModelIDs()[0] == "changed" {
		t.Fatal("SupportedProbeModelIDs returned shared backing storage")
	}
}

func newProbeServer(t *testing.T, delay time.Duration, content string) *httptest.Server {
	t.Helper()
	wsServer := websocket.Server{
		Handler: websocket.Handler(func(ws *websocket.Conn) {
			var prompt string
			if err := websocket.Message.Receive(ws, &prompt); err != nil {
				return
			}
			if delay > 0 {
				time.Sleep(delay)
			}
			_ = websocket.Message.Send(ws, []byte(`{"type":"start","content":""}`))
			_ = websocket.Message.Send(ws, []byte(`{"type":"middle","content":"`+content+`"}`))
			_ = websocket.Message.Send(ws, []byte(`{"type":"end","content":""}`))
		}),
	}
	return httptest.NewServer(wsServer)
}

func probeTestConfig(serverURL string) Config {
	return Config{
		Endpoint: strings.Replace(serverURL, "http://", "ws://", 1),
		Origin:   serverURL,
		Token:    "test-token",
		UseAPI:   "test-use-api",
		Model:    "GLM-5.2",
	}
}

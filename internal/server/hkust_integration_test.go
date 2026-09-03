package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/net/websocket"
)

func TestHKUSTUpstreamServesOpenAIChatAndResponsesWithoutDeepSeekAccounts(t *testing.T) {
	wsServer := httptest.NewServer(websocket.Server{
		Handshake: func(_ *websocket.Config, _ *http.Request) error {
			return nil
		},
		Handler: websocket.Handler(func(ws *websocket.Conn) {
			var prompt string
			if err := websocket.Message.Receive(ws, &prompt); err != nil {
				t.Errorf("receive upstream prompt: %v", err)
				return
			}
			if !strings.Contains(prompt, "hello") {
				t.Errorf("upstream prompt = %q, want it to contain hello", prompt)
			}

			content := "OK"
			if strings.Contains(prompt, "lookup_secret") {
				content = `<|DSML|tool_calls>
<|DSML|invoke name="lookup_secret">
<|DSML|parameter name="key"><![CDATA[vault-7]]></|DSML|parameter>
</|DSML|invoke>
</|DSML|tool_calls>`
			}
			for _, frame := range [][]byte{
				[]byte(`{"type":"start","content":""}`),
				[]byte(`{"type":"middle","content":` + mustJSONQuote(content) + `}`),
				[]byte(`{"type":"end","content":""}`),
			} {
				if err := websocket.Message.Send(ws, frame); err != nil {
					t.Errorf("send upstream frame: %v", err)
					return
				}
			}
		}),
	})
	defer wsServer.Close()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	configJSON := `{
  "keys": ["proxy-key"],
  "current_input_file": {"enabled": true, "min_chars": 0},
  "auto_delete": {"mode": "none"}
}`
	if err := os.WriteFile(configPath, []byte(configJSON), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("DS2API_CONFIG_JSON", "")
	t.Setenv("DS2API_CONFIG_PATH", configPath)
	t.Setenv("DS2API_CHAT_HISTORY_PATH", filepath.Join(tempDir, "chat_history.json"))
	t.Setenv("HKUST_TOKEN", "school-token")
	t.Setenv("HKUST_USE_API", "school-use-api")
	t.Setenv("HKUST_WS_URL", strings.Replace(wsServer.URL, "http://", "ws://", 1))
	t.Setenv("HKUST_ORIGIN", wsServer.URL)
	t.Setenv("HKUST_MODEL", "DeepSeek-V4-Pro-conv")

	app, err := NewApp()
	if err != nil {
		t.Fatalf("NewApp() error = %v", err)
	}

	t.Run("chat completions", func(t *testing.T) {
		body := `{"model":"deepseek-v4-pro","messages":[{"role":"user","content":"hello"}]}`
		recorder := serveHKUSTTestRequest(t, app.Router, "/v1/chat/completions", body)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
		var response map[string]any
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode chat response: %v; body=%s", err, recorder.Body.String())
		}
		if !strings.Contains(recorder.Body.String(), "OK") {
			t.Fatalf("chat response missing OK: %s", recorder.Body.String())
		}
	})

	t.Run("responses", func(t *testing.T) {
		body := `{"model":"deepseek-v4-pro","input":"hello"}`
		recorder := serveHKUSTTestRequest(t, app.Router, "/v1/responses", body)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
		var response map[string]any
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode responses response: %v; body=%s", err, recorder.Body.String())
		}
		if !strings.Contains(recorder.Body.String(), "OK") {
			t.Fatalf("responses response missing OK: %s", recorder.Body.String())
		}
	})

	t.Run("prompt tool call", func(t *testing.T) {
		body := `{
  "model":"deepseek-v4-pro",
  "messages":[{"role":"user","content":"hello, use lookup_secret for vault-7"}],
  "tools":[{
    "type":"function",
    "function":{
      "name":"lookup_secret",
      "description":"look up a secret by key",
      "parameters":{
        "type":"object",
        "properties":{"key":{"type":"string"}},
        "required":["key"]
      }
    }
  }],
  "tool_choice":"required"
}`
		recorder := serveHKUSTTestRequest(t, app.Router, "/v1/chat/completions", body)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
		var response map[string]any
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode tool response: %v; body=%s", err, recorder.Body.String())
		}
		choices, _ := response["choices"].([]any)
		if len(choices) != 1 {
			t.Fatalf("choices = %#v", response["choices"])
		}
		choice, _ := choices[0].(map[string]any)
		if choice["finish_reason"] != "tool_calls" {
			t.Fatalf("finish_reason = %#v, body=%s", choice["finish_reason"], recorder.Body.String())
		}
		message, _ := choice["message"].(map[string]any)
		toolCalls, _ := message["tool_calls"].([]any)
		if len(toolCalls) != 1 || !strings.Contains(recorder.Body.String(), "lookup_secret") || !strings.Contains(recorder.Body.String(), "vault-7") {
			t.Fatalf("tool_calls = %#v, body=%s", toolCalls, recorder.Body.String())
		}
	})
}

func mustJSONQuote(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func serveHKUSTTestRequest(t *testing.T, handler http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer proxy-key")
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}

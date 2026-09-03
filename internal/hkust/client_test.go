package hkust

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/net/websocket"
)

func TestCallCompletionBridgesHKUSTWebSocketToDeepSeekSSE(t *testing.T) {
	promptCh := make(chan string, 1)
	queryCh := make(chan map[string]string, 1)
	serverErrCh := make(chan error, 1)

	wsServer := websocket.Server{
		Handshake: func(_ *websocket.Config, req *http.Request) error {
			query := req.URL.Query()
			queryCh <- map[string]string{
				"subjectGuid": query.Get("subjectGuid"),
				"model":       query.Get("model"),
				"token":       query.Get("token"),
				"useApi":      query.Get("useApi"),
				"thinking":    query.Get("thinking"),
			}
			return nil
		},
		Handler: websocket.Handler(func(ws *websocket.Conn) {
			var prompt string
			if err := websocket.Message.Receive(ws, &prompt); err != nil {
				serverErrCh <- err
				return
			}
			promptCh <- prompt
			frames := [][]byte{
				[]byte(`{"type":"start","content":""}`),
				[]byte(`{"type":"middle","content":"<thi"}`),
				[]byte(`{"type":"middle","content":"nk>hidden"}`),
				[]byte(`{"type":"middle","content":"</thi"}`),
				[]byte(`{"type":"middle","content":"nk>OK"}`),
				[]byte(`{"type":"end","content":""}`),
			}
			for _, frame := range frames {
				if err := websocket.Message.Send(ws, frame); err != nil {
					serverErrCh <- err
					return
				}
			}
		}),
	}

	server := httptest.NewServer(wsServer)
	defer server.Close()

	client := NewClient(Config{
		Endpoint: strings.Replace(server.URL, "http://", "ws://", 1),
		Origin:   server.URL,
		Token:    "school-token",
		UseAPI:   "school-use-api",
		Model:    "DeepSeek-V4-Pro-conv",
	})
	client.heartbeatInterval = time.Hour

	resp, err := client.CallCompletion(context.Background(), nil, map[string]any{"prompt": "hello"}, "", 1)
	if err != nil {
		t.Fatalf("CallCompletion() error = %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Fatalf("response body close: %v", err)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, `"p":"response/thinking_content","v":"hidden"`) {
		t.Fatalf("response missing thinking segment: %s", text)
	}
	if !strings.Contains(text, `"p":"response/content","v":"OK"`) {
		t.Fatalf("response missing text segment: %s", text)
	}
	if !strings.Contains(text, `"p":"response/status","v":"FINISHED"`) {
		t.Fatalf("response missing finished status: %s", text)
	}

	select {
	case prompt := <-promptCh:
		if prompt != "hello" {
			t.Fatalf("upstream prompt = %q, want hello", prompt)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for upstream prompt")
	}

	select {
	case query := <-queryCh:
		if _, err := uuid.Parse(query["subjectGuid"]); err != nil {
			t.Fatalf("subjectGuid = %q, want UUID", query["subjectGuid"])
		}
		if query["model"] != "DeepSeek-V4-Pro-conv" || query["token"] != "school-token" || query["useApi"] != "school-use-api" {
			t.Fatalf("unexpected upstream query: %#v", query)
		}
		if query["thinking"] != "false" {
			t.Fatalf("thinking = %q, want false", query["thinking"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for upstream query")
	}

	select {
	case err := <-serverErrCh:
		t.Fatalf("websocket server error: %v", err)
	default:
	}
}

func TestThinkSplitterAcrossFrameBoundaries(t *testing.T) {
	splitter := thinkSplitter{}
	var got []streamSegment
	got = append(got, splitter.Feed("before<thi")...)
	got = append(got, splitter.Feed("nk>inside</thi")...)
	got = append(got, splitter.Feed("nk>after")...)
	got = append(got, splitter.Flush()...)

	want := []streamSegment{
		{Text: "before", Thinking: false},
		{Text: "inside", Thinking: true},
		{Text: "after", Thinking: false},
	}
	if len(got) != len(want) {
		t.Fatalf("segments = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("segment[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}

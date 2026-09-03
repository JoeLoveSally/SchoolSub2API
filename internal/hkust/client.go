package hkust

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/net/websocket"

	"ds2api/internal/auth"
	"ds2api/internal/config"
	dsclient "ds2api/internal/deepseek/client"
)

const defaultHeartbeatInterval = 10 * time.Second

var ErrFileUploadUnsupported = errors.New("HKUST web chat upstream does not support file upload")

type Client struct {
	cfg               Config
	heartbeatInterval time.Duration
}

func NewClient(cfg Config) *Client {
	return &Client{cfg: cfg, heartbeatInterval: defaultHeartbeatInterval}
}

func (c *Client) CreateSession(_ context.Context, _ *auth.RequestAuth, _ int) (string, error) {
	return uuid.NewString(), nil
}

func (c *Client) GetPow(_ context.Context, _ *auth.RequestAuth, _ int) (string, error) {
	return "", nil
}

func (c *Client) UploadFile(_ context.Context, _ *auth.RequestAuth, _ dsclient.UploadFileRequest, _ int) (*dsclient.UploadFileResult, error) {
	return nil, ErrFileUploadUnsupported
}

func (c *Client) DeleteSessionForToken(_ context.Context, _ string, sessionID string) (*dsclient.DeleteSessionResult, error) {
	return &dsclient.DeleteSessionResult{SessionID: sessionID, Success: true}, nil
}

func (c *Client) DeleteAllSessionsForToken(_ context.Context, _ string) error {
	return nil
}

func (c *Client) CallCompletion(ctx context.Context, _ *auth.RequestAuth, payload map[string]any, _ string, _ int) (*http.Response, error) {
	prompt, _ := payload["prompt"].(string)
	if strings.TrimSpace(prompt) == "" {
		return nil, errors.New("HKUST completion prompt is empty")
	}
	prompt = adaptPromptForWebChat(prompt)

	endpoint, err := c.completionURL(uuid.NewString())
	if err != nil {
		return nil, err
	}
	wsConfig, err := websocket.NewConfig(endpoint, c.cfg.Origin)
	if err != nil {
		return nil, fmt.Errorf("build HKUST websocket config: %w", err)
	}
	if wsConfig.Header == nil {
		wsConfig.Header = make(http.Header)
	}
	wsConfig.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/152.0.0.0 Safari/537.36")

	ws, err := websocket.DialConfig(wsConfig)
	if err != nil {
		return nil, c.redactedError("dial HKUST websocket", err)
	}
	if err := websocket.Message.Send(ws, prompt); err != nil {
		if closeErr := ws.Close(); closeErr != nil {
			config.Logger.Warn("[hkust] websocket close after send failure failed", "error", c.redactedError("close websocket", closeErr))
		}
		return nil, c.redactedError("send HKUST prompt", err)
	}

	reader, writer := io.Pipe()
	go c.pumpStream(ctx, ws, writer)

	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     fmt.Sprintf("%d %s", http.StatusOK, http.StatusText(http.StatusOK)),
		Header: http.Header{
			"Content-Type": []string{"text/event-stream; charset=utf-8"},
		},
		Body: reader,
	}, nil
}

func (c *Client) completionURL(subjectGUID string) (string, error) {
	u, err := url.Parse(c.cfg.Endpoint)
	if err != nil {
		return "", fmt.Errorf("parse HKUST websocket endpoint: %w", err)
	}
	if u.Scheme != "ws" && u.Scheme != "wss" {
		return "", fmt.Errorf("HKUST websocket endpoint must use ws or wss")
	}
	q := u.Query()
	q.Set("subjectGuid", subjectGUID)
	q.Set("model", c.cfg.Model)
	q.Set("token", c.cfg.Token)
	q.Set("useApi", c.cfg.UseAPI)
	q.Set("thinking", "false")
	q.Set("enableThinking", "")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

type hkustFrame struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

type streamSegment struct {
	Text     string
	Thinking bool
}

func (c *Client) pumpStream(ctx context.Context, ws *websocket.Conn, writer *io.PipeWriter) {
	var closeOnce sync.Once
	closeWS := func(reason string) {
		closeOnce.Do(func() {
			if err := ws.Close(); err != nil {
				config.Logger.Warn("[hkust] websocket close failed", "reason", reason, "error", c.redactedError("close websocket", err))
			}
		})
	}
	defer closeWS("stream_done")

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			closeWS("context_cancelled")
		case <-done:
		}
	}()
	go c.heartbeat(ws, done, closeWS)

	splitter := thinkSplitter{}
	protocolFilter := protocolBoundaryFilter{}
	for {
		var raw []byte
		if err := websocket.Message.Receive(ws, &raw); err != nil {
			if ctx.Err() != nil {
				closePipeWithError(writer, ctx.Err())
				return
			}
			closePipeWithError(writer, c.redactedError("receive HKUST websocket frame", err))
			return
		}
		message := string(raw)
		switch strings.TrimSpace(message) {
		case "heartbeat-pong", "done":
			continue
		}

		var frame hkustFrame
		if err := json.Unmarshal(raw, &frame); err != nil {
			config.Logger.Debug("[hkust] ignoring non-JSON websocket frame")
			continue
		}
		switch frame.Type {
		case "start":
			continue
		case "middle":
			safeContent := protocolFilter.Feed(frame.Content)
			if err := writeSegments(writer, splitter.Feed(safeContent)); err != nil {
				closePipeWithError(writer, err)
				return
			}
		case "end":
			if tail := protocolFilter.Flush(); tail != "" {
				if err := writeSegments(writer, splitter.Feed(tail)); err != nil {
					closePipeWithError(writer, err)
					return
				}
			}
			if err := writeSegments(writer, splitter.Flush()); err != nil {
				closePipeWithError(writer, err)
				return
			}
			if err := writeSSE(writer, "response/status", "FINISHED"); err != nil {
				closePipeWithError(writer, err)
				return
			}
			if err := writer.Close(); err != nil {
				config.Logger.Warn("[hkust] response pipe close failed", "error", err)
			}
			return
		}
	}
}

func closePipeWithError(writer *io.PipeWriter, streamErr error) {
	if writer == nil {
		return
	}
	if err := writer.CloseWithError(streamErr); err != nil {
		config.Logger.Warn("[hkust] response pipe close failed", "error", err)
	}
}

func (c *Client) heartbeat(ws *websocket.Conn, done <-chan struct{}, closeWS func(string)) {
	interval := c.heartbeatInterval
	if interval <= 0 {
		interval = defaultHeartbeatInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			if err := websocket.Message.Send(ws, "heartbeat-ping"); err != nil {
				config.Logger.Warn("[hkust] heartbeat send failed", "error", c.redactedError("send heartbeat", err))
				closeWS("heartbeat_failed")
				return
			}
		}
	}
}

func writeSegments(writer io.Writer, segments []streamSegment) error {
	for _, segment := range segments {
		if segment.Text == "" {
			continue
		}
		path := "response/content"
		if segment.Thinking {
			path = "response/thinking_content"
		}
		if err := writeSSE(writer, path, segment.Text); err != nil {
			return err
		}
	}
	return nil
}

func writeSSE(writer io.Writer, path string, value any) error {
	payload, err := json.Marshal(map[string]any{"p": path, "v": value})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(writer, "data: %s\n\n", payload)
	return err
}

type thinkSplitter struct {
	thinking bool
	pending  string
}

func (s *thinkSplitter) Feed(chunk string) []streamSegment {
	data := s.pending + chunk
	s.pending = ""
	segments := make([]streamSegment, 0, 2)

	for data != "" {
		tag := "<think>"
		if s.thinking {
			tag = "</think>"
		}
		if idx := strings.Index(data, tag); idx >= 0 {
			if idx > 0 {
				segments = append(segments, streamSegment{Text: data[:idx], Thinking: s.thinking})
			}
			data = data[idx+len(tag):]
			s.thinking = !s.thinking
			continue
		}

		keep := longestSuffixMatchingTagPrefix(data, tag)
		emitEnd := len(data) - keep
		if emitEnd > 0 {
			segments = append(segments, streamSegment{Text: data[:emitEnd], Thinking: s.thinking})
		}
		if keep > 0 {
			s.pending = data[emitEnd:]
		}
		break
	}
	return segments
}

func (s *thinkSplitter) Flush() []streamSegment {
	if s.pending == "" {
		return nil
	}
	segment := streamSegment{Text: s.pending, Thinking: s.thinking}
	s.pending = ""
	return []streamSegment{segment}
}

func longestSuffixMatchingTagPrefix(data, tag string) int {
	max := len(tag) - 1
	if len(data) < max {
		max = len(data)
	}
	for n := max; n > 0; n-- {
		if strings.HasSuffix(data, tag[:n]) {
			return n
		}
	}
	return 0
}

func (c *Client) redactedError(operation string, err error) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	if c.cfg.Token != "" {
		message = strings.ReplaceAll(message, c.cfg.Token, "[REDACTED]")
	}
	if c.cfg.UseAPI != "" {
		message = strings.ReplaceAll(message, c.cfg.UseAPI, "[REDACTED]")
	}
	return fmt.Errorf("%s: %s", operation, message)
}

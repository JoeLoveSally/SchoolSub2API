package hkust

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

const (
	defaultProbeTimeout = 10 * time.Second
	probeMarker         = "JOEJOEPROXY_OK"
	maxProbeBodyBytes   = 64 << 10
)

var supportedProbeModels = []string{
	"GLM-5.2",
	"DeepSeek-V4-Flash-conv",
	"DeepSeek-V4-Pro-conv",
	"Kimi-K3",
}

type ModelProbeResult struct {
	Model     string `json:"model"`
	Status    string `json:"status"`
	LatencyMS int64  `json:"latency_ms"`
	Message   string `json:"message,omitempty"`
}

func SupportedProbeModelIDs() []string {
	return append([]string(nil), supportedProbeModels...)
}

func (c *Client) ProbeModels(ctx context.Context, models []string, timeout time.Duration) []ModelProbeResult {
	if len(models) == 0 {
		models = SupportedProbeModelIDs()
	}
	results := make([]ModelProbeResult, len(models))
	var wg sync.WaitGroup
	wg.Add(len(models))
	for i, model := range models {
		i, model := i, model
		go func() {
			defer wg.Done()
			results[i] = c.ProbeModel(ctx, model, timeout)
		}()
	}
	wg.Wait()
	return results
}

func (c *Client) ProbeModel(ctx context.Context, model string, timeout time.Duration) ModelProbeResult {
	model = strings.TrimSpace(model)
	started := time.Now()
	result := ModelProbeResult{Model: model}
	finish := func(status, message string) ModelProbeResult {
		result.Status = status
		result.Message = strings.TrimSpace(message)
		result.LatencyMS = time.Since(started).Milliseconds()
		return result
	}

	if !isSupportedProbeModel(model) {
		return finish("failed", fmt.Sprintf("unsupported HKUST probe model %q", model))
	}
	if timeout <= 0 {
		timeout = defaultProbeTimeout
	}

	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	probeClient := *c
	probeClient.cfg.Model = model
	type completionResult struct {
		body io.ReadCloser
		err  error
	}
	completionCh := make(chan completionResult, 1)
	go func() {
		resp, err := probeClient.CallCompletion(
			probeCtx,
			nil,
			map[string]any{"prompt": "Reply with exactly JOEJOEPROXY_OK and nothing else."},
			"",
			1,
		)
		if err != nil {
			completionCh <- completionResult{err: err}
			return
		}
		if resp == nil || resp.Body == nil {
			completionCh <- completionResult{err: errors.New("HKUST probe returned an empty response")}
			return
		}
		completionCh <- completionResult{body: resp.Body}
	}()

	var body io.ReadCloser
	select {
	case <-probeCtx.Done():
		return finish("timeout", "HKUST probe timed out")
	case completion := <-completionCh:
		if completion.err != nil {
			if errors.Is(probeCtx.Err(), context.DeadlineExceeded) || errors.Is(completion.err, context.DeadlineExceeded) {
				return finish("timeout", "HKUST probe timed out")
			}
			return finish("failed", completion.err.Error())
		}
		body = completion.body
	}
	defer func() { _ = body.Close() }()

	type readResult struct {
		data []byte
		err  error
	}
	readCh := make(chan readResult, 1)
	go func() {
		data, err := io.ReadAll(io.LimitReader(body, maxProbeBodyBytes))
		readCh <- readResult{data: data, err: err}
	}()

	select {
	case <-probeCtx.Done():
		_ = body.Close()
		return finish("timeout", "HKUST probe timed out")
	case read := <-readCh:
		if read.err != nil {
			if errors.Is(probeCtx.Err(), context.DeadlineExceeded) || errors.Is(read.err, context.DeadlineExceeded) {
				return finish("timeout", "HKUST probe timed out")
			}
			return finish("failed", read.err.Error())
		}
		if !strings.Contains(strings.ToUpper(string(read.data)), probeMarker) {
			return finish("failed", "HKUST returned a response, but it did not pass the probe marker check")
		}
		return finish("available", "")
	}
}

func isSupportedProbeModel(model string) bool {
	for _, candidate := range supportedProbeModels {
		if strings.EqualFold(candidate, model) {
			return true
		}
	}
	return false
}

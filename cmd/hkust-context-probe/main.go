package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/net/websocket"

	"ds2api/internal/config"
	"ds2api/internal/hkust"
)

const (
	probeUserAgent       = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/152.0.0.0 Safari/537.36"
	probeHeartbeat       = 10 * time.Second
	defaultProbeTargets  = "70000,96000,128000,192000,256000"
	defaultProbeTimeout  = 4 * time.Minute
	smokeProbeTokenUnits = 1024
)
var contextLimitPattern = regexp.MustCompile(`(?i)maximum\s+context\s+length\s+is\s+([0-9][0-9,]*)\s+tokens`)

type modelSpec struct {
	name    string
	aliases []string
}
type probeOutcome string

const (
	probePass     probeOutcome = "PASS"
	probeOverflow probeOutcome = "OVERFLOW"
	probeError    probeOutcome = "ERROR"
)
type probeResult struct {
	outcome probeOutcome
	limit   int
	detail  string
}
type modelSummary struct {
	name          string
	resolvedModel string
	highestPass   int
	reportedLimit int
	status        probeOutcome
	detail        string
}

func main() {
	var (
		targetsFlag = flag.String("targets", defaultProbeTargets, "comma-separated approximate input-token targets")
		modelsFlag  = flag.String("models", "", "comma-separated exact HKUST web-chat model IDs; overrides defaults")
		includeGPT  = flag.Bool("include-gpt", false, "also probe gpt-5.6-luna (may consume the limited monthly web-chat allowance)")
		timeoutFlag = flag.Duration("timeout", defaultProbeTimeout, "timeout for each WebSocket probe")
	)
	flag.Parse()
	if err := config.LoadDotEnv(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: load .env: %v\n", err)
	}
	cfg, enabled, err := hkust.LoadConfigFromEnv()
	if err != nil {
		fatalf("load HKUST config: %v", err)
	}
	if !enabled {
		fatalf("HKUST upstream is not configured; set HKUST_TOKEN and HKUST_USE_API (or place them in .env)")
	}
	targets, err := parseTargets(*targetsFlag)
	if err != nil {
		fatalf("invalid -targets: %v", err)
	}
	if *timeoutFlag <= 0 {
		fatalf("-timeout must be positive")
	}
	models := defaultModels(*includeGPT)
	if strings.TrimSpace(*modelsFlag) != "" {
		models = exactModels(*modelsFlag)
	}
	fmt.Println("HKUST web-chat context probe")
	fmt.Printf("endpoint: %s\n", cfg.Endpoint)
	fmt.Printf("targets:  %v approximate token units\n", targets)
	fmt.Println("note: one probe unit is one repeated ' hello' segment; reported upstream limits are authoritative when present.")
	if !*includeGPT && strings.TrimSpace(*modelsFlag) == "" {
		fmt.Println("note: gpt-5.6-luna is skipped by default because its web-chat allowance is limited; add -include-gpt to test it.")
	}
	fmt.Println()
	summaries := make([]modelSummary, 0, len(models))
	for _, spec := range models {
		summary := probeModelSeries(cfg, spec, targets, *timeoutFlag)
		summaries = append(summaries, summary)
		fmt.Println()
	}
	printSummary(summaries)
}

func defaultModels(includeGPT bool) []modelSpec {
	models := []modelSpec{
		{name: "DeepSeek-V4-Pro", aliases: []string{"DeepSeek-V4-Pro-conv", "DeepSeek-V4-Pro"}},
		{name: "DeepSeek-V4-Flash", aliases: []string{"DeepSeek-V4-Flash-conv", "DeepSeek-V4-Flash"}},
		{name: "GLM-5.2", aliases: []string{"GLM-5.2", "GLM-5.2-conv"}},
		{name: "Kimi-K3", aliases: []string{"Kimi-K3", "Kimi-K3-conv"}},
	}
	if includeGPT {
		models = append(models, modelSpec{name: "GPT-5.6", aliases: []string{"gpt-5.6-luna", "GPT-5.6", "GPT-5.6-conv"}})
	}
	return models
}

func exactModels(raw string) []modelSpec {
	parts := strings.Split(raw, ",")
	out := make([]modelSpec, 0, len(parts))
	for _, part := range parts {
		model := strings.TrimSpace(part)
		if model != "" {
			out = append(out, modelSpec{name: model, aliases: []string{model}})
		}
	}
	return out
}

func parseTargets(raw string) ([]int, error) {
	parts := strings.Split(raw, ",")
	out := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		value, err := strconv.Atoi(part)
		if err != nil || value <= 0 {
			return nil, fmt.Errorf("%q is not a positive integer", part)
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("at least one target is required")
	}
	return out, nil
}

func probeModelSeries(cfg hkust.Config, spec modelSpec, targets []int, timeout time.Duration) modelSummary {
	fmt.Printf("[%s]\n", spec.name)
	resolved, smoke := resolveModelAlias(cfg, spec.aliases, timeout)
	if resolved == "" {
		detail := smoke.detail
		if detail == "" {
			detail = "no alias accepted"
		}
		fmt.Printf("  model resolution: ERROR  %s\n", detail)
		return modelSummary{name: spec.name, status: probeError, detail: detail}
	}
	fmt.Printf("  resolved model: %s\n", resolved)
	summary := modelSummary{name: spec.name, resolvedModel: resolved, status: probePass}
	for _, target := range targets {
		started := time.Now()
		result := probeOnce(cfg, resolved, target, timeout)
		elapsed := time.Since(started).Round(time.Millisecond)
		switch result.outcome {
		case probePass:
			summary.highestPass = target
			fmt.Printf("  %7d: PASS      (%s)\n", target, elapsed)
		case probeOverflow:
			summary.status = probeOverflow
			summary.reportedLimit = result.limit
			if result.limit > 0 {
				fmt.Printf("  %7d: OVERFLOW  (%s) upstream_limit=%d\n", target, elapsed, result.limit)
			} else {
				fmt.Printf("  %7d: OVERFLOW  (%s)\n", target, elapsed)
			}
			return summary
		default:
			summary.status = probeError
			summary.detail = result.detail
			fmt.Printf("  %7d: ERROR     (%s) %s\n", target, elapsed, result.detail)
			return summary
		}
	}
	return summary
}

func resolveModelAlias(cfg hkust.Config, aliases []string, timeout time.Duration) (string, probeResult) {
	last := probeResult{outcome: probeError, detail: "no model alias attempted"}
	for _, alias := range aliases {
		result := probeOnce(cfg, alias, smokeProbeTokenUnits, timeout)
		if result.outcome == probePass || result.outcome == probeOverflow {
			return alias, result
		}
		last = result
	}
	return "", last
}

func probeOnce(cfg hkust.Config, model string, tokenUnits int, timeout time.Duration) probeResult {
	endpoint, err := buildProbeURL(cfg, model)
	if err != nil {
		return probeResult{outcome: probeError, detail: redact(cfg, err.Error())}
	}
	wsConfig, err := websocket.NewConfig(endpoint, cfg.Origin)
	if err != nil {
		return probeResult{outcome: probeError, detail: redact(cfg, err.Error())}
	}
	if wsConfig.Header == nil {
		wsConfig.Header = make(http.Header)
	}
	wsConfig.Header.Set("User-Agent", probeUserAgent)
	ws, err := websocket.DialConfig(wsConfig)
	if err != nil {
		return probeResult{outcome: probeError, detail: redact(cfg, err.Error())}
	}
	defer func() { _ = ws.Close() }()
	_ = ws.SetDeadline(time.Now().Add(timeout))
	if err := websocket.Message.Send(ws, makeProbePrompt(tokenUnits)); err != nil {
		return probeResult{outcome: probeError, detail: redact(cfg, err.Error())}
	}
	done := make(chan struct{})
	defer close(done)
	go heartbeat(ws, done)
	var diagnostic strings.Builder
	for {
		var raw []byte
		if err := websocket.Message.Receive(ws, &raw); err != nil {
			detail := diagnostic.String() + " " + err.Error()
			if limit, ok := contextOverflow(detail); ok {
				return probeResult{outcome: probeOverflow, limit: limit}
			}
			return probeResult{outcome: probeError, detail: shorten(redact(cfg, detail))}
		}
		message := string(raw)
		trimmed := strings.TrimSpace(message)
		if trimmed == "heartbeat-pong" || trimmed == "done" {
			continue
		}
		if limit, ok := contextOverflow(message); ok {
			return probeResult{outcome: probeOverflow, limit: limit}
		}
		appendDiagnostic(&diagnostic, message)
		var frame map[string]any
		if err := json.Unmarshal(raw, &frame); err != nil {
			continue
		}
		frameType, _ := frame["type"].(string)
		switch strings.ToLower(strings.TrimSpace(frameType)) {
		case "end":
			return probeResult{outcome: probePass}
		case "error", "failed", "failure":
			return probeResult{outcome: probeError, detail: shorten(redact(cfg, frameDiagnostic(frame)))}
		}
	}
}

func buildProbeURL(cfg hkust.Config, model string) (string, error) {
	u, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return "", err
	}
	if u.Scheme != "ws" && u.Scheme != "wss" {
		return "", fmt.Errorf("HKUST websocket endpoint must use ws or wss")
	}
	q := u.Query()
	q.Set("subjectGuid", uuid.NewString())
	q.Set("model", model)
	q.Set("token", cfg.Token)
	q.Set("useApi", cfg.UseAPI)
	q.Set("thinking", "false")
	q.Set("enableThinking", "")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func makeProbePrompt(tokenUnits int) string {
	return "Context-window probe. Ignore the repeated filler and reply only OK after reading it.\n" +
		strings.Repeat(" hello", tokenUnits) + "\nReply only OK."
}

func heartbeat(ws *websocket.Conn, done <-chan struct{}) {
	ticker := time.NewTicker(probeHeartbeat)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			_ = websocket.Message.Send(ws, "heartbeat-ping")
		}
	}
}

func contextOverflow(message string) (int, bool) {
	lower := strings.ToLower(message)
	isOverflow := strings.Contains(lower, "contextwindowexceeded") ||
		strings.Contains(lower, "context window exceeded") ||
		strings.Contains(lower, "context length exceeded") ||
		strings.Contains(lower, "prompt is too long") ||
		(strings.Contains(lower, "maximum context length") && strings.Contains(lower, "token"))
	if !isOverflow {
		return 0, false
	}
	match := contextLimitPattern.FindStringSubmatch(message)
	if len(match) != 2 {
		return 0, true
	}
	value, err := strconv.Atoi(strings.ReplaceAll(match[1], ",", ""))
	if err != nil {
		return 0, true
	}
	return value, true
}

func frameDiagnostic(frame map[string]any) string {
	for _, key := range []string{"message", "content", "error", "detail"} {
		if value, ok := frame[key]; ok {
			if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
				return text
			}
			encoded, _ := json.Marshal(value)
			if len(encoded) > 0 && string(encoded) != "null" {
				return string(encoded)
			}
		}
	}
	encoded, _ := json.Marshal(frame)
	return string(encoded)
}

func appendDiagnostic(dst *strings.Builder, message string) {
	if dst.Len() >= 4096 {
		return
	}
	remaining := 4096 - dst.Len()
	if len(message) > remaining {
		message = message[:remaining]
	}
	dst.WriteString(message)
	dst.WriteByte('\n')
}

func redact(cfg hkust.Config, message string) string {
	if cfg.Token != "" {
		message = strings.ReplaceAll(message, cfg.Token, "[REDACTED]")
	}
	if cfg.UseAPI != "" {
		message = strings.ReplaceAll(message, cfg.UseAPI, "[REDACTED]")
	}
	return message
}

func shorten(message string) string {
	message = strings.Join(strings.Fields(message), " ")
	if len(message) <= 300 {
		return message
	}
	return message[:300] + "..."
}

func printSummary(summaries []modelSummary) {
	fmt.Println("Summary")
	fmt.Printf("%-22s %-24s %-13s %-15s %s\n", "MODEL", "RESOLVED_ID", "HIGHEST_PASS", "UPSTREAM_LIMIT", "STATUS")
	for _, item := range summaries {
		highest := "-"
		if item.highestPass > 0 {
			highest = strconv.Itoa(item.highestPass)
		}
		limit := "-"
		if item.reportedLimit > 0 {
			limit = strconv.Itoa(item.reportedLimit)
		}
		resolved := item.resolvedModel
		if resolved == "" {
			resolved = "-"
		}
		status := string(item.status)
		if item.detail != "" && item.status == probeError {
			status += ": " + shorten(item.detail)
		}
		fmt.Printf("%-22s %-24s %-13s %-15s %s\n", item.name, resolved, highest, limit, status)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}

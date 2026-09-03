# HKUST(GZ) Web Chat Upstream

This fork can route the normal DS2API API surfaces through the HKUST(GZ) AIGC web-chat WebSocket instead of the official DeepSeek web backend.

The goal is intentionally narrow: reuse DS2API's existing OpenAI/Responses/Claude/Gemini normalization, PromptCompat, tool-call parsing, and streaming renderers while replacing only the upstream transport.

## Enable

Set both credentials in the process environment or `.env`:

```bash
export HKUST_TOKEN='...'
export HKUST_USE_API='...'
```

Optional overrides:

```bash
export HKUST_MODEL='DeepSeek-V4-Pro-conv'
export HKUST_WS_URL='wss://aigc.hkust-gz.edu.cn/chat/new'
export HKUST_ORIGIN='https://aigc.hkust-gz.edu.cn'
```

Real HKUST credentials must never be committed to the repository.

When either `HKUST_TOKEN`, `HKUST_USE_API`, or `HKUST_WS_URL` is present, HKUST mode is considered configured. Missing required credentials then fail server startup instead of silently falling back to the official DeepSeek backend.

## Client authentication

HKUST credentials are upstream credentials and are not exposed to API clients.

Clients continue to authenticate with a normal DS2API API key from `config.json` / `config.example.json`:

```bash
curl http://127.0.0.1:5001/v1/chat/completions \
  -H 'Authorization: Bearer <DS2API_KEY>' \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "deepseek-v4-pro",
    "messages": [{"role": "user", "content": "只回复 OK"}]
  }'
```

In HKUST mode the API-key resolver does not acquire a DeepSeek account from the normal account pool.

## Transport mapping

The upstream request is:

```text
wss://aigc.hkust-gz.edu.cn/chat/new
  ?subjectGuid=<fresh uuid>
  &model=DeepSeek-V4-Pro-conv
  &token=<HKUST_TOKEN>
  &useApi=<HKUST_USE_API>
  &thinking=false
  &enableThinking=
```

DS2API still builds the complete PromptCompat `FinalPrompt`, but the HKUST transport does not send DeepSeek's internal chat-template control tokens verbatim. The school endpoint accepts a normal web-chat text message and applies its own conversation template, so raw markers such as `<|System|>`, `<|User|>`, `<|Tool|>`, and `<|end▁of▁sentence|>` would double-template the request and can cause transcript/tool-result replay.

At the HKUST boundary those role markers are converted into a natural-language transcript such as `System instructions follow:`, `User message follows:`, and `Result from the previously requested tool follows:`. The final synthetic `<|Assistant|>` cue is converted into an explicit request for only the next assistant response. DSML tool-call syntax is preserved unchanged so the existing prompt-based tool parser continues to work.

The prompt preamble tells the model that transcript scaffolding is context only and must not be reproduced. A defensive stream filter also stops output if the model starts replaying either DeepSeek's internal role markers or known transcript scaffolding at an output line boundary. Legacy bracket labels such as `[TOOL RESULT]` are filtered as well so older adapted turns cannot leak into Claude Code output.

Observed upstream frames:

```json
{"type":"start","content":""}
{"type":"middle","content":"<think>"}
{"type":"middle","content":"..."}
{"type":"middle","content":"</think>"}
{"type":"middle","content":"answer"}
{"type":"end","content":""}
```

The adapter converts them into the DeepSeek SSE shape already consumed by `internal/sse`:

```text
data: {"p":"response/thinking_content","v":"..."}

data: {"p":"response/content","v":"answer"}

data: {"p":"response/status","v":"FINISHED"}
```

`<think>` / `</think>` markers are parsed across WebSocket frame boundaries. `heartbeat-ping` is sent periodically and `heartbeat-pong` / `done` control frames are ignored.

## Session semantics

DS2API requests already contain the complete PromptCompat conversation context. Therefore every upstream completion uses a fresh `subjectGuid` instead of reusing HKUST server-side history. This avoids duplicating history when OpenAI/Codex clients resend prior turns.

The synthetic DS2API session ID is only an internal compatibility value. HKUST session deletion is currently a no-op because no delete protocol has been verified.

## Tool calling

No native HKUST tool-call channel has been observed. Tool calling therefore continues to use DS2API's existing PromptCompat / prompt-based tool-call protocol and parser.

The upstream adapter deliberately does not implement its own tool schema or parser. This keeps Codex/Responses/Claude behavior in the shared DS2API compatibility path.

Claude's top-level `system` field may be either a string or an array of text content blocks. Both forms are normalized before PromptCompat construction so Claude Code environment metadata such as its working directory is retained when tool instructions are merged.

DSML is preserved byte-for-byte by the HKUST prompt adaptation layer. Regression coverage includes shell commands containing significant whitespace such as `git checkout HEAD -- src/config/config.yaml`; if a generated command is malformed before parsing, that remains a model-generation error rather than an adapter rewrite.

## Current limitations

- HKUST file upload is not implemented; requests requiring upstream file upload return an unsupported error.
- Web search and the official DeepSeek web-only session/account management paths are not provided by the HKUST transport.
- The adapter currently uses the verified webpage `thinking=false` connection mode. `<think>` output is still separated when it appears.
- `subjectGuid` conversations may remain stored on the school service because a deletion endpoint has not been verified.
- The school WebSocket is a private web protocol and may change without compatibility guarantees.

## Initial verification

Run the normal repository gates before opening a PR:

```bash
./scripts/lint.sh
./tests/scripts/check-refactor-line-gate.sh
./tests/scripts/run-unit-all.sh
npm run build --prefix webui
```

Then verify both surfaces:

```bash
curl -N http://127.0.0.1:5001/v1/chat/completions \
  -H 'Authorization: Bearer <DS2API_KEY>' \
  -H 'Content-Type: application/json' \
  -d '{"model":"deepseek-v4-pro","stream":true,"messages":[{"role":"user","content":"只回复 CHAT_OK"}]}'
```

```bash
curl -N http://127.0.0.1:5001/v1/responses \
  -H 'Authorization: Bearer <DS2API_KEY>' \
  -H 'Content-Type: application/json' \
  -d '{"model":"deepseek-v4-pro","stream":true,"input":"只回复 RESPONSES_OK"}'
```

After those pass, verify a prompt-based tool loop through `/v1/responses` or `/v1/messages`, then run a real Claude Code tool sequence such as `pwd` -> `Read` -> `Edit` -> `git diff` -> restore. Client-visible output must not contain DeepSeek role markers, transcript scaffolding, or replayed tool-result labels.

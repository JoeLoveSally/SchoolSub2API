# JoeJoeProxy macOS App

JoeJoeProxy is the macOS launcher for the HKUST Web Chat upstream. It bundles the Go proxy, listens on `127.0.0.1:5001`, checks HKUST model connectivity with real WebSocket requests, and generates a WorkBuddy `models.json` snippet.

## Supported models

The app exposes the HKUST WebSocket model IDs that have been verified in this project:

| App option | HKUST upstream ID | WorkBuddy ID | WorkBuddy input budget | Notes |
| --- | --- | --- | ---: | --- |
| GLM-5.2 | `GLM-5.2` | `HKUST-GLM-5.2` | 220,000 | Default; verified deployment limit |
| DeepSeek V4 Flash | `DeepSeek-V4-Flash-conv` | `HKUST-DeepSeek-V4-Flash` | 262,144 | Verified deployment limit; live availability is probed at runtime |
| DeepSeek V4 Pro | `DeepSeek-V4-Pro-conv` | `HKUST-DeepSeek-V4-Pro` | 65,535 | Verified deployment limit |
| Kimi K3 | `Kimi-K3` | `HKUST-Kimi-K3` | 262,144 | Experimental web-chat route; conservative WorkBuddy budget |

WorkBuddy IDs deliberately use an uppercase `HKUST-` prefix so they do not collide with WorkBuddy built-in model IDs. The generated WorkBuddy entry uses the OpenAI-compatible custom API format and points to `http://127.0.0.1:5001/v1/chat/completions`.

The selected model is passed to the bundled proxy as `HKUST_MODEL`. WorkBuddy `lite` and `reasoning` variants both point to the selected `HKUST-...` model ID.

## Live connectivity probe

Availability labels are not hard-coded. JoeJoeProxy invokes a probe mode in the bundled Go binary and sends a small real request to each HKUST model. The models are checked in parallel with a per-model timeout. A model is marked `available` only when the response contains the expected probe marker; a normal-looking response containing an upstream error therefore does not count as available.

The UI supports two probe paths:

1. Before starting the proxy, enter `token` and `useApi`, then click **检测全部模型**.
2. While the proxy is running, click **重新检测** to refresh all model states without restarting the proxy.

Starting or switching a model also performs a fresh live probe first. When switching from a running model, the existing proxy remains running until the requested model passes the probe.

## User flow

1. Open `JoeJoeProxy.app`.
2. Enter your own HKUST `token` and `useApi` values.
3. Optionally click **检测全部模型** to see current connectivity.
4. Choose a model; GLM-5.2 is the default.
5. Click **启动本地代理**.
6. JoeJoeProxy performs a live HKUST probe, starts the bundled proxy, and validates the local custom model alias.
7. On success, copy the generated WorkBuddy configuration into `~/.codebuddy/models.json`.

HKUST credentials are passed only to the child proxy/probe processes and are not written to the generated WorkBuddy configuration. The independent local proxy API key is stored in `~/Library/Application Support/JoeJoeProxy/local_api_key` with `0600` permissions.

## Build

Builds require macOS because the bundle uses SwiftUI and macOS packaging tools:

```bash
bash scripts/build-macos-app.sh
```

GitHub Actions builds both Apple Silicon and Intel artifacts from the `mac-app` branch.

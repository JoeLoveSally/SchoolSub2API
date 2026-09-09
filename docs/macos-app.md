# JoeJoeProxy macOS App

JoeJoeProxy is the macOS launcher for the HKUST Web Chat upstream. It bundles the Go proxy, listens on `127.0.0.1:5001`, validates the selected HKUST model with a live request, and generates a WorkBuddy `models.json` snippet.

## Supported models

The app currently exposes the HKUST WebSocket models that have been verified in this project:

| App option | HKUST upstream ID | WorkBuddy input budget | Notes |
| --- | --- | ---: | --- |
| DeepSeek V4 Flash | `DeepSeek-V4-Flash-conv` | 262,144 | Recommended; verified deployment limit |
| GLM-5.2 | `GLM-5.2` | 220,000 | Verified deployment limit; slower on long prompts |
| DeepSeek V4 Pro | `DeepSeek-V4-Pro-conv` | 65,535 | Verified deployment limit |
| Kimi K3 | `Kimi-K3` | 262,144 | Experimental web-chat route; conservative WorkBuddy budget |

The selected model is passed to the bundled proxy as `HKUST_MODEL`. WorkBuddy `lite` and `reasoning` variants both point to the selected model.

## User flow

1. Open `JoeJoeProxy.app`.
2. Enter your own HKUST `token` and `useApi` values.
3. Choose a model; Flash is the default.
4. Click **启动本地代理**.
5. JoeJoeProxy starts the bundled proxy and validates the selected model with a live request.
6. On success, copy the generated WorkBuddy configuration into `~/.codebuddy/models.json`.

HKUST credentials are passed only to the child proxy process and are not written to the generated WorkBuddy configuration.

## Build

Builds require macOS because the bundle uses SwiftUI and macOS packaging tools:

```bash
bash scripts/build-macos-app.sh
```

GitHub Actions builds both Apple Silicon and Intel artifacts from the `mac-app` branch.

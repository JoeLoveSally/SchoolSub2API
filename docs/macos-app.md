# macOS 本地代理 App

`mac-app` 分支提供一个轻量 macOS SwiftUI 启动器，用来把已验证的 HKUST Web Chat 上游封装成本机 OpenAI 兼容代理。

## 用户流程

1. 打开 `SchoolSub2API.app`。
2. 手动输入 HKUST `token` 和 `useApi`。
3. 点击“启动本地代理”。
4. App 会启动内置 `ds2api`，固定使用 `DeepSeek-V4-Flash-conv`，并只监听 `127.0.0.1:5001`。
5. App 先检查 `/healthz`，随后发送一次真实的 `/v1/chat/completions` 请求验证 HKUST 凭据和 Flash 上游。
6. 验证成功后弹窗提示，并展示可复制的 WorkBuddy `models.json` 配置。

HKUST `token` / `useApi` 只通过子进程环境变量传给本机 `ds2api`，不会写入 App 配置文件。用于本机 Harness 鉴权的随机 API key 会保存在 macOS Keychain 中，以便 WorkBuddy 配置在 App 重启后保持稳定。

## 固定模型

当前 App 只使用：

- Upstream model: `DeepSeek-V4-Flash-conv`
- WorkBuddy model id: `deepseek-v4-flash`
- `maxInputTokens`: `262144`
- `maxOutputTokens`: `4096`
- `lite`: `deepseek-v4-flash`
- `reasoning`: `deepseek-v4-flash`

也就是说主模型、lite 和 reasoning 全部走 Flash。

## WorkBuddy

App 启动成功后会生成完整配置，引导用户粘贴到：

```text
~/.codebuddy/models.json
```

WorkBuddy 当前自定义模型接口使用 OpenAI Chat Completions 完整 URL，因此生成的 endpoint 为：

```text
http://127.0.0.1:5001/v1/chat/completions
```

如果用户已经有 `~/.codebuddy/models.json`，应合并生成配置里的 `models` / `availableModels`，不要覆盖已有的其他模型。

## 本地构建

要求：

- macOS 13+
- Go 1.26+
- Swift 5.9+ / Xcode Command Line Tools

在仓库根目录执行：

```bash
bash scripts/build-macos-app.sh
```

当前机器是 Apple Silicon 时生成：

```text
dist/macos/arm64/SchoolSub2API.app
dist/macos/arm64/SchoolSub2API-macos-arm64.zip
```

Intel Mac 对应 `x86_64` 目录。

## GitHub Actions 构建

`.github/workflows/macos-app.yml` 会分别在：

- `macos-15`：arm64
- `macos-15-intel`：x86_64

构建两个可下载 Artifact。

当前构建只做 ad-hoc codesign，没有 Apple Developer ID notarization。因此直接分发给其他 Mac 时仍可能遇到 Gatekeeper 的“未识别开发者”提示。正式外部分发时应增加 Developer ID 签名和 Apple notarization。

## 运行时安全边界

macOS App 设置：

```text
DS2API_BIND_HOST=127.0.0.1
PORT=5001
DS2API_AUTO_BUILD_WEBUI=0
DS2API_ENV_WRITEBACK=0
HKUST_MODEL=DeepSeek-V4-Flash-conv
```

因此 App 版默认不会把代理暴露到局域网，也不会把运行时 `DS2API_CONFIG_JSON` 回写到磁盘。

import AppKit
import SwiftUI

struct ContentView: View {
    @EnvironmentObject private var controller: ProxyController
    @State private var token = ""
    @State private var useAPI = ""
    @State private var showSuccess = false

    var body: some View {
        VStack(alignment: .leading, spacing: 18) {
            header
            Divider()

            if controller.status == .running {
                runningView
            } else {
                credentialView
            }
        }
        .padding(24)
        .frame(width: 720, height: controller.status == .running ? 690 : 390)
        .alert("代理启动成功", isPresented: $showSuccess) {
            Button("好的", role: .cancel) {}
        } message: {
            Text("已通过 HKUST DeepSeek V4 Flash 实际请求验证，本地代理正在 \(controller.proxyBaseURL) 运行。")
        }
    }

    private var header: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("SchoolSub2API")
                .font(.title.bold())
            Text("HKUST Web Chat → 本地 OpenAI 兼容代理")
                .foregroundStyle(.secondary)
            Text("固定使用 DeepSeek V4 Flash · 262,144 输入上下文")
                .font(.caption)
                .foregroundStyle(.secondary)
        }
    }

    private var credentialView: some View {
        VStack(alignment: .leading, spacing: 14) {
            Text("HKUST 凭据")
                .font(.headline)

            SecureField("token", text: $token)
                .textFieldStyle(.roundedBorder)
            SecureField("useApi", text: $useAPI)
                .textFieldStyle(.roundedBorder)

            Text("凭据只传给本机代理进程，本应用不会把 token / useApi 写入配置文件。")
                .font(.caption)
                .foregroundStyle(.secondary)

            statusLine

            HStack {
                Spacer()
                Button(controller.status == .starting ? "正在验证…" : "启动本地代理") {
                    Task {
                        let succeeded = await controller.start(token: token, useAPI: useAPI)
                        if succeeded {
                            token = ""
                            useAPI = ""
                            showSuccess = true
                        }
                    }
                }
                .buttonStyle(.borderedProminent)
                .disabled(
                    controller.status == .starting ||
                    token.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ||
                    useAPI.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
                )
            }
        }
    }

    private var runningView: some View {
        VStack(alignment: .leading, spacing: 14) {
            statusLine

            GroupBox("本地代理") {
                Grid(alignment: .leading, horizontalSpacing: 16, verticalSpacing: 8) {
                    GridRow {
                        Text("Endpoint")
                            .foregroundStyle(.secondary)
                        Text(controller.proxyBaseURL)
                            .textSelection(.enabled)
                    }
                    GridRow {
                        Text("Model")
                            .foregroundStyle(.secondary)
                        Text("DeepSeek-V4-Flash-conv")
                    }
                    GridRow {
                        Text("Bind")
                            .foregroundStyle(.secondary)
                        Text("127.0.0.1:5001（仅本机）")
                    }
                }
                .padding(8)
            }

            Text("WorkBuddy 配置")
                .font(.headline)
            Text("把下面内容粘贴到 \(WorkBuddyConfig.configPath)。如果文件里已有其他模型，请合并 models / availableModels，不要直接覆盖原有配置。WorkBuddy 会热重载 models.json。")
                .font(.caption)
                .foregroundStyle(.secondary)

            TextEditor(text: .constant(controller.workBuddyConfig))
                .font(.system(.body, design: .monospaced))
                .frame(minHeight: 300)
                .overlay(
                    RoundedRectangle(cornerRadius: 6)
                        .stroke(Color.secondary.opacity(0.25))
                )

            HStack {
                Button("复制配置") {
                    copy(controller.workBuddyConfig)
                }
                Button("复制配置路径") {
                    copy(WorkBuddyConfig.configPath)
                }
                Spacer()
                Button("停止代理", role: .destructive) {
                    controller.stop()
                }
            }
        }
    }

    @ViewBuilder
    private var statusLine: some View {
        HStack(alignment: .top, spacing: 8) {
            if controller.status == .starting {
                ProgressView()
                    .controlSize(.small)
            } else {
                Circle()
                    .fill(statusColor)
                    .frame(width: 9, height: 9)
                    .padding(.top, 5)
            }
            Text(controller.statusMessage)
                .font(.callout)
                .foregroundStyle(controller.status == .failed ? .red : .primary)
                .textSelection(.enabled)
        }
    }

    private var statusColor: Color {
        switch controller.status {
        case .running:
            return .green
        case .failed:
            return .red
        case .starting:
            return .orange
        case .idle:
            return .secondary
        }
    }

    private func copy(_ text: String) {
        let pasteboard = NSPasteboard.general
        pasteboard.clearContents()
        pasteboard.setString(text, forType: .string)
    }
}

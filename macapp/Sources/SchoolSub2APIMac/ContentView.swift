import AppKit
import SwiftUI

struct ContentView: View {
    @EnvironmentObject private var controller: ProxyController
    @State private var token = ""
    @State private var useAPI = ""
    @State private var selectedModel: HKUSTModel = .defaultModel
    @State private var showSuccess = false

    var body: some View {
        VStack(alignment: .leading, spacing: 18) {
            header
            Divider()

            if showsRunningView {
                runningView
            } else {
                credentialView
            }
        }
        .padding(24)
        .frame(width: 720, height: showsRunningView ? 830 : 540)
        .alert("代理启动成功", isPresented: $showSuccess) {
            Button("好的", role: .cancel) {}
        } message: {
            Text("已通过 HKUST \(controller.activeModel?.displayName ?? selectedModel.displayName) 实时连通性检测，本地代理正在 \(controller.proxyBaseURL) 运行。")
        }
    }

    private var showsRunningView: Bool {
        controller.status == .running ||
        (controller.status == .starting && controller.activeModel != nil)
    }

    private var header: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("JoeJoeProxy")
                .font(.title.bold())
            Text("HKUST Web Chat → 本地 OpenAI 兼容代理")
                .foregroundStyle(.secondary)
            Text("默认 GLM-5.2；模型状态由 HKUST 实时检测，不再写死。")
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

            Text("模型")
                .font(.headline)
            Picker("模型", selection: $selectedModel) {
                ForEach(HKUSTModel.allCases) { model in
                    Text(modelMenuLabel(model)).tag(model)
                }
            }
            .labelsHidden()
            .pickerStyle(.menu)
            .disabled(controller.status == .starting || controller.isProbingModels)

            Text(selectedModel.detail)
                .font(.caption)
                .foregroundStyle(.secondary)

            modelProbeRow(showRefreshButton: false)

            Text("凭据只传给本机代理/检测进程，本应用不会把 token / useApi 写入配置文件。")
                .font(.caption)
                .foregroundStyle(.secondary)

            statusLine

            HStack {
                Button(controller.isProbingModels ? "正在检测…" : "检测全部模型") {
                    Task {
                        await controller.checkModels(token: token, useAPI: useAPI)
                    }
                }
                .disabled(
                    controller.isProbingModels ||
                    controller.status == .starting ||
                    token.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ||
                    useAPI.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
                )

                Spacer()

                Button(controller.status == .starting ? "正在启动…" : "启动本地代理") {
                    Task {
                        let succeeded = await controller.start(
                            token: token,
                            useAPI: useAPI,
                            model: selectedModel
                        )
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
                    controller.isProbingModels ||
                    token.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ||
                    useAPI.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
                )
            }
        }
    }

    private var runningView: some View {
        let activeModel = controller.activeModel ?? selectedModel
        return VStack(alignment: .leading, spacing: 14) {
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
                        Text(activeModel.upstreamID)
                    }
                    GridRow {
                        Text("WorkBuddy ID")
                            .foregroundStyle(.secondary)
                        Text(activeModel.workBuddyID)
                            .textSelection(.enabled)
                    }
                    GridRow {
                        Text("Context")
                            .foregroundStyle(.secondary)
                        Text("\(activeModel.maxInputTokens.formatted()) tokens")
                    }
                    GridRow {
                        Text("Bind")
                            .foregroundStyle(.secondary)
                        Text("127.0.0.1:5001（仅本机）")
                    }
                }
                .padding(8)
            }

            GroupBox("切换模型") {
                VStack(alignment: .leading, spacing: 10) {
                    Picker("模型", selection: $selectedModel) {
                        ForEach(HKUSTModel.allCases) { model in
                            Text(modelMenuLabel(model)).tag(model)
                        }
                    }
                    .pickerStyle(.menu)
                    .disabled(controller.status == .starting || controller.isProbingModels)

                    Text(selectedModel.detail)
                        .font(.caption)
                        .foregroundStyle(.secondary)

                    modelProbeRow(showRefreshButton: true)

                    HStack {
                        Spacer()
                        Button(controller.status == .starting ? "正在切换…" : "切换模型并重启代理") {
                            Task {
                                let succeeded = await controller.switchModel(to: selectedModel)
                                if succeeded {
                                    showSuccess = true
                                }
                            }
                        }
                        .buttonStyle(.borderedProminent)
                        .disabled(
                            controller.status == .starting ||
                            controller.isProbingModels ||
                            selectedModel == controller.activeModel
                        )
                    }
                }
                .padding(8)
            }

            Text("WorkBuddy 配置")
                .font(.headline)
            Text("把下面内容粘贴到 \(WorkBuddyConfig.configPath)。如果文件里已有其他模型，请合并 models / availableModels，不要直接覆盖原有配置。WorkBuddy 会热重载 models.json。模型 ID 使用独立的 HKUST- 前缀，避免与内置模型冲突。")
                .font(.caption)
                .foregroundStyle(.secondary)

            TextEditor(text: .constant(controller.workBuddyConfig))
                .font(.system(.body, design: .monospaced))
                .frame(minHeight: 240)
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
    private func modelProbeRow(showRefreshButton: Bool) -> some View {
        HStack(spacing: 8) {
            Circle()
                .fill(probeColor(controller.probeState(for: selectedModel)))
                .frame(width: 9, height: 9)

            Text("\(selectedModel.displayName)：\(controller.probeState(for: selectedModel).label)")
                .font(.caption)

            if let lastProbeAt = controller.lastProbeAt {
                Text("·")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                Text(lastProbeAt, style: .time)
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }

            Spacer()

            if showRefreshButton {
                Button(controller.isProbingModels ? "检测中…" : "重新检测") {
                    Task {
                        await controller.refreshModelStatus()
                    }
                }
                .controlSize(.small)
                .disabled(controller.isProbingModels || controller.status == .starting)
            }
        }

        Text(controller.probeMessage)
            .font(.caption)
            .foregroundStyle(.secondary)
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

    private func probeColor(_ state: ModelProbeState) -> Color {
        switch state {
        case .available:
            return .green
        case .timeout, .failed:
            return .red
        case .checking:
            return .orange
        case .unchecked:
            return .secondary
        }
    }

    private func modelMenuLabel(_ model: HKUSTModel) -> String {
        "\(model.pickerLabel) · \(controller.probeState(for: model).label)"
    }

    private func copy(_ text: String) {
        let pasteboard = NSPasteboard.general
        pasteboard.clearContents()
        pasteboard.setString(text, forType: .string)
    }
}

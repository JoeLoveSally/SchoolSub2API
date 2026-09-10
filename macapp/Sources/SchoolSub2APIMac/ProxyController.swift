import Combine
import Foundation

enum ProxyStatus {
    case idle
    case starting
    case running
    case failed
}

enum ModelProbeState: Equatable {
    case unchecked
    case checking
    case available(latencyMS: Int)
    case timeout
    case failed(String)

    var isAvailable: Bool {
        if case .available = self {
            return true
        }
        return false
    }

    var label: String {
        switch self {
        case .unchecked:
            return "未检测"
        case .checking:
            return "检测中…"
        case .available(let latencyMS):
            if latencyMS >= 1_000 {
                return String(format: "可用 · %.1fs", Double(latencyMS) / 1_000.0)
            }
            return "可用 · \(latencyMS)ms"
        case .timeout:
            return "超时"
        case .failed:
            return "失败"
        }
    }
}

final class ProxyController: ObservableObject {
    static let shared = ProxyController()

    static let port = 5001
    private static let probeOutputPrefix = "JOEJOEPROXY_PROBE_JSON="

    @Published private(set) var status: ProxyStatus = .idle
    @Published private(set) var statusMessage = "Enter your HKUST credentials to start the local proxy."
    @Published private(set) var workBuddyConfig = ""
    @Published private(set) var localAPIKey = ""
    @Published private(set) var activeModel: HKUSTModel?
    @Published private(set) var modelProbeStates: [HKUSTModel: ModelProbeState] = Dictionary(
        uniqueKeysWithValues: HKUSTModel.allCases.map { ($0, ModelProbeState.unchecked) }
    )
    @Published private(set) var lastProbeAt: Date?
    @Published private(set) var isProbingModels = false
    @Published private(set) var probeMessage = "尚未检测 HKUST 模型连通性。"

    private var process: Process?
    private var outputPipe: Pipe?
    private var processLog = ""
    private var cachedToken = ""
    private var cachedUseAPI = ""

    private init() {}

    var proxyBaseURL: String {
        "http://127.0.0.1:\(Self.port)"
    }

    func probeState(for model: HKUSTModel) -> ModelProbeState {
        modelProbeStates[model] ?? .unchecked
    }

    @MainActor
    func checkModels(token: String, useAPI: String) async {
        let cleanToken = token.trimmingCharacters(in: .whitespacesAndNewlines)
        let cleanUseAPI = useAPI.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !cleanToken.isEmpty, !cleanUseAPI.isEmpty else {
            probeMessage = "请先填写 token 和 useApi。"
            return
        }
        do {
            try await performModelProbe(token: cleanToken, useAPI: cleanUseAPI)
        } catch {
            probeMessage = "检测失败：\(shorten(error.localizedDescription))"
        }
    }

    @MainActor
    func refreshModelStatus() async {
        guard !cachedToken.isEmpty, !cachedUseAPI.isEmpty else {
            probeMessage = "HKUST 凭据已不在内存中，无法重新检测。"
            return
        }
        do {
            try await performModelProbe(token: cachedToken, useAPI: cachedUseAPI)
        } catch {
            probeMessage = "检测失败：\(shorten(error.localizedDescription))"
        }
    }

    @MainActor
    func start(token: String, useAPI: String, model: HKUSTModel) async -> Bool {
        let cleanToken = token.trimmingCharacters(in: .whitespacesAndNewlines)
        let cleanUseAPI = useAPI.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !cleanToken.isEmpty, !cleanUseAPI.isEmpty else {
            status = .failed
            statusMessage = "Token and useApi are both required."
            return false
        }

        let previousProcess = process
        let previousModel = activeModel
        let wasRunning = status == .running && previousModel != nil && previousProcess?.isRunning == true

        status = .starting
        statusMessage = wasRunning
            ? "正在实时检测 HKUST \(model.displayName)，当前代理会保持运行直到检测通过。"
            : "正在实时检测 HKUST 模型连通性…"
        processLog = ""

        do {
            try await performModelProbe(token: cleanToken, useAPI: cleanUseAPI)
            guard probeState(for: model).isAvailable else {
                let reason = probeFailureDescription(for: model)
                if wasRunning, let previousModel {
                    status = .running
                    statusMessage = "HKUST \(model.displayName) 检测未通过（\(reason)）；当前 \(previousModel.displayName) 代理保持运行。"
                } else {
                    status = .failed
                    statusMessage = "HKUST \(model.displayName) 检测未通过：\(reason)"
                }
                return false
            }

            stopProcess()
            if !wasRunning {
                activeModel = nil
                workBuddyConfig = ""
            }

            let apiKey = try LocalAPIKeyStore.loadOrCreate()
            let child = try makeProcess(
                token: cleanToken,
                useAPI: cleanUseAPI,
                apiKey: apiKey,
                model: model
            )
            process = child
            try child.run()
            installTerminationHandler(for: child)

            try await waitUntilHealthy(process: child)
            try await verifyLocalModelAlias(apiKey: apiKey, model: model)

            guard child.isRunning, process === child else {
                throw launcherError("Proxy process exited during startup validation.")
            }

            cachedToken = cleanToken
            cachedUseAPI = cleanUseAPI
            localAPIKey = apiKey
            workBuddyConfig = try WorkBuddyConfig.render(apiKey: apiKey, port: Self.port, model: model)
            activeModel = model
            status = .running
            statusMessage = "代理启动成功。HKUST \(model.displayName) 已通过实时连通性检测。"
            return true
        } catch {
            let detail = usefulFailureDetail(error)
            if wasRunning,
               let previousProcess,
               process === previousProcess,
               previousProcess.isRunning {
                status = .running
                statusMessage = "检测或切换失败，当前代理保持运行。\n\(detail)"
                return false
            }

            stopProcess()
            activeModel = nil
            workBuddyConfig = ""
            localAPIKey = ""
            status = .failed
            statusMessage = detail
            return false
        }
    }

    @MainActor
    func switchModel(to model: HKUSTModel) async -> Bool {
        guard status == .running, let currentModel = activeModel else {
            status = .failed
            statusMessage = "The proxy must be running before switching models."
            return false
        }
        if currentModel == model {
            statusMessage = "HKUST \(model.displayName) is already active."
            return true
        }
        guard !cachedToken.isEmpty, !cachedUseAPI.isEmpty else {
            status = .failed
            statusMessage = "HKUST credentials are no longer available in memory. Stop the proxy and enter them again."
            return false
        }
        return await start(token: cachedToken, useAPI: cachedUseAPI, model: model)
    }

    @MainActor
    func stop() {
        stopProcess()
        activeModel = nil
        cachedToken = ""
        cachedUseAPI = ""
        status = .idle
        statusMessage = "Proxy stopped. Enter your HKUST credentials to start it again."
        workBuddyConfig = ""
        localAPIKey = ""
        modelProbeStates = Dictionary(
            uniqueKeysWithValues: HKUSTModel.allCases.map { ($0, ModelProbeState.unchecked) }
        )
        lastProbeAt = nil
        probeMessage = "尚未检测 HKUST 模型连通性。"
    }

    @MainActor
    private func performModelProbe(token: String, useAPI: String) async throws {
        guard !isProbingModels else {
            throw launcherError("HKUST model probe is already running.")
        }

        isProbingModels = true
        for model in HKUSTModel.allCases {
            modelProbeStates[model] = .checking
        }
        probeMessage = "正在并行检测 \(HKUSTModel.allCases.count) 个 HKUST 模型…"
        defer { isProbingModels = false }

        do {
            let output = try await runProbeHelper(token: token, useAPI: useAPI)
            let envelope = try decodeProbeEnvelope(output.data)
            if let message = envelope.error, !message.isEmpty {
                throw launcherError(message)
            }
            if output.terminationStatus != 0 && envelope.results.isEmpty {
                throw launcherError("HKUST probe helper exited with status \(output.terminationStatus).")
            }

            var updated: [HKUSTModel: ModelProbeState] = Dictionary(
                uniqueKeysWithValues: HKUSTModel.allCases.map {
                    ($0, ModelProbeState.failed("未收到该模型的检测结果"))
                }
            )
            for result in envelope.results {
                guard let model = HKUSTModel.from(upstreamID: result.model) else {
                    continue
                }
                switch result.status.lowercased() {
                case "available":
                    updated[model] = .available(latencyMS: max(0, result.latencyMS))
                case "timeout":
                    updated[model] = .timeout
                default:
                    updated[model] = .failed(result.message ?? "HKUST probe failed")
                }
            }
            modelProbeStates = updated
            lastProbeAt = Date()
            probeMessage = "HKUST 模型实时检测完成。"
        } catch {
            for model in HKUSTModel.allCases where modelProbeStates[model] == .checking {
                modelProbeStates[model] = .failed(error.localizedDescription)
            }
            lastProbeAt = Date()
            throw error
        }
    }

    private func runProbeHelper(token: String, useAPI: String) async throws -> ProbeProcessOutput {
        guard let binaryURL = Bundle.main.resourceURL?.appendingPathComponent("ds2api"),
              FileManager.default.isExecutableFile(atPath: binaryURL.path) else {
            throw launcherError("Bundled ds2api executable is missing. Rebuild the macOS app bundle.")
        }

        let child = Process()
        child.executableURL = binaryURL
        child.arguments = ["hkust-probe", "--timeout=10s"] + HKUSTModel.allCases.map(\.upstreamID)

        var environment = ProcessInfo.processInfo.environment
        environment["HKUST_TOKEN"] = token
        environment["HKUST_USE_API"] = useAPI
        environment["HKUST_MODEL"] = HKUSTModel.defaultModel.upstreamID
        child.environment = environment

        let pipe = Pipe()
        child.standardOutput = pipe
        child.standardError = pipe

        return try await withCheckedThrowingContinuation { continuation in
            child.terminationHandler = { process in
                let data = pipe.fileHandleForReading.readDataToEndOfFile()
                continuation.resume(
                    returning: ProbeProcessOutput(
                        data: data,
                        terminationStatus: process.terminationStatus
                    )
                )
            }
            do {
                try child.run()
            } catch {
                continuation.resume(throwing: error)
            }
        }
    }

    private func decodeProbeEnvelope(_ data: Data) throws -> ProbeEnvelope {
        guard let text = String(data: data, encoding: .utf8) else {
            throw launcherError("Unable to decode HKUST probe helper output.")
        }
        guard let line = text
            .split(whereSeparator: \.isNewline)
            .last(where: { $0.hasPrefix(Self.probeOutputPrefix) }) else {
            throw launcherError("HKUST probe helper returned no structured result: \(shorten(text))")
        }
        let encoded = String(line.dropFirst(Self.probeOutputPrefix.count))
        guard let payload = Data(base64Encoded: encoded) else {
            throw launcherError("Unable to decode HKUST probe result payload.")
        }
        return try JSONDecoder().decode(ProbeEnvelope.self, from: payload)
    }

    private func probeFailureDescription(for model: HKUSTModel) -> String {
        switch probeState(for: model) {
        case .unchecked:
            return "未检测"
        case .checking:
            return "仍在检测"
        case .available:
            return "可用"
        case .timeout:
            return "连接超时"
        case .failed(let message):
            return shorten(message)
        }
    }

    @MainActor
    private func makeProcess(
        token: String,
        useAPI: String,
        apiKey: String,
        model: HKUSTModel
    ) throws -> Process {
        guard let binaryURL = Bundle.main.resourceURL?.appendingPathComponent("ds2api"),
              FileManager.default.isExecutableFile(atPath: binaryURL.path) else {
            throw launcherError("Bundled ds2api executable is missing. Rebuild the macOS app bundle.")
        }

        let runtimeDir = try runtimeDirectory()
        let configJSON = try LocalProxyConfig.render(apiKey: apiKey)

        var environment = ProcessInfo.processInfo.environment
        environment["HKUST_TOKEN"] = token
        environment["HKUST_USE_API"] = useAPI
        environment["HKUST_MODEL"] = model.upstreamID
        environment["PORT"] = String(Self.port)
        environment["DS2API_BIND_HOST"] = "127.0.0.1"
        environment["DS2API_CONFIG_JSON"] = configJSON
        environment["DS2API_ENV_WRITEBACK"] = "0"
        environment["DS2API_AUTO_BUILD_WEBUI"] = "0"
        environment["DS2API_ADMIN_KEY"] = apiKey
        environment["DS2API_CONFIG_PATH"] = runtimeDir.appendingPathComponent("config.json").path
        environment["DS2API_CHAT_HISTORY_PATH"] = runtimeDir.appendingPathComponent("chat_history.json").path

        let pipe = Pipe()
        pipe.fileHandleForReading.readabilityHandler = { [weak self] handle in
            let data = handle.availableData
            guard !data.isEmpty, let text = String(data: data, encoding: .utf8) else {
                return
            }
            DispatchQueue.main.async {
                self?.appendProcessLog(text)
            }
        }
        outputPipe = pipe

        let child = Process()
        child.executableURL = binaryURL
        child.currentDirectoryURL = runtimeDir
        child.environment = environment
        child.standardOutput = pipe
        child.standardError = pipe
        return child
    }

    @MainActor
    private func installTerminationHandler(for child: Process) {
        child.terminationHandler = { [weak self] endedProcess in
            DispatchQueue.main.async {
                guard let self, self.process === endedProcess else {
                    return
                }
                self.process = nil
                self.outputPipe?.fileHandleForReading.readabilityHandler = nil
                self.outputPipe = nil
                if self.status == .running || self.status == .starting {
                    self.activeModel = nil
                    self.workBuddyConfig = ""
                    self.localAPIKey = ""
                    self.status = .failed
                    self.statusMessage = self.usefulFailureDetail(
                        self.launcherError("Proxy process exited unexpectedly.")
                    )
                }
            }
        }
    }

    @MainActor
    private func waitUntilHealthy(process child: Process) async throws {
        let deadline = Date().addingTimeInterval(12)
        while Date() < deadline {
            guard child.isRunning, process === child else {
                throw launcherError("Proxy process exited before becoming ready. Port 5001 may already be in use.")
            }
            if await healthCheck() {
                return
            }
            try await Task.sleep(nanoseconds: 300_000_000)
        }
        throw launcherError("Local proxy did not become ready within 12 seconds.")
    }

    private func healthCheck() async -> Bool {
        guard let url = URL(string: "\(proxyBaseURL)/healthz") else {
            return false
        }
        var request = URLRequest(url: url)
        request.timeoutInterval = 1
        do {
            let (_, response) = try await URLSession.shared.data(for: request)
            return (response as? HTTPURLResponse)?.statusCode == 200
        } catch {
            return false
        }
    }

    private func verifyLocalModelAlias(apiKey: String, model: HKUSTModel) async throws {
        let escapedModel = model.workBuddyID.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed) ?? model.workBuddyID
        guard let url = URL(string: "\(proxyBaseURL)/v1/models/\(escapedModel)") else {
            throw launcherError("Invalid local model validation URL.")
        }
        var request = URLRequest(url: url)
        request.timeoutInterval = 3
        request.setValue("Bearer \(apiKey)", forHTTPHeaderField: "Authorization")
        let (data, response) = try await URLSession.shared.data(for: request)
        guard let http = response as? HTTPURLResponse, http.statusCode == 200 else {
            let detail = String(data: data, encoding: .utf8) ?? "local model alias validation failed"
            throw launcherError("Local WorkBuddy model ID \(model.workBuddyID) was not accepted: \(shorten(detail))")
        }
    }

    @MainActor
    private func runtimeDirectory() throws -> URL {
        guard let base = FileManager.default.urls(
            for: .applicationSupportDirectory,
            in: .userDomainMask
        ).first else {
            throw launcherError("Unable to locate Application Support directory.")
        }
        let dir = base.appendingPathComponent("SchoolSub2API", isDirectory: true)
        try FileManager.default.createDirectory(
            at: dir,
            withIntermediateDirectories: true
        )
        return dir
    }

    @MainActor
    private func stopProcess() {
        let child = process
        process = nil
        outputPipe?.fileHandleForReading.readabilityHandler = nil
        outputPipe = nil
        if let child, child.isRunning {
            child.terminate()
        }
    }

    @MainActor
    private func appendProcessLog(_ text: String) {
        processLog.append(text)
        if processLog.count > 4_000 {
            processLog = String(processLog.suffix(4_000))
        }
    }

    @MainActor
    private func usefulFailureDetail(_ error: Error) -> String {
        let base = error.localizedDescription
        let log = processLog.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !log.isEmpty else {
            return base
        }
        return "\(base)\n\nProxy log:\n\(shorten(log))"
    }

    private func shorten(_ text: String) -> String {
        let compact = text.split(whereSeparator: { $0.isWhitespace }).joined(separator: " ")
        if compact.count <= 900 {
            return compact
        }
        return String(compact.prefix(900)) + "..."
    }

    private func launcherError(_ message: String) -> NSError {
        NSError(
            domain: "SchoolSub2API.MacLauncher",
            code: 1,
            userInfo: [NSLocalizedDescriptionKey: message]
        )
    }
}

private struct ProbeProcessOutput {
    let data: Data
    let terminationStatus: Int32
}

private struct ProbeEnvelope: Decodable {
    let results: [ProbeResult]
    let error: String?
}

private struct ProbeResult: Decodable {
    let model: String
    let status: String
    let latencyMS: Int
    let message: String?

    enum CodingKeys: String, CodingKey {
        case model
        case status
        case latencyMS = "latency_ms"
        case message
    }
}

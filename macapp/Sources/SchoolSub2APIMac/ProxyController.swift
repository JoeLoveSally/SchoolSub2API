import Combine
import Foundation

enum ProxyStatus {
    case idle
    case starting
    case running
    case failed
}

final class ProxyController: ObservableObject {
    static let shared = ProxyController()

    static let port = 5001

    @Published private(set) var status: ProxyStatus = .idle
    @Published private(set) var statusMessage = "Enter your HKUST credentials to start the local proxy."
    @Published private(set) var workBuddyConfig = ""
    @Published private(set) var localAPIKey = ""
    @Published private(set) var activeModel: HKUSTModel?

    private var process: Process?
    private var outputPipe: Pipe?
    private var processLog = ""
    private var cachedToken = ""
    private var cachedUseAPI = ""

    private init() {}

    var proxyBaseURL: String {
        "http://127.0.0.1:\(Self.port)"
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

        let wasRunning = status == .running && activeModel != nil
        stopProcess()
        if !wasRunning {
            activeModel = nil
            workBuddyConfig = ""
        }
        status = .starting
        statusMessage = wasRunning
            ? "Switching local proxy to HKUST \(model.displayName) and validating it..."
            : "Starting local proxy and validating HKUST \(model.displayName)..."
        processLog = ""

        do {
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
            try await verifyHKUST(apiKey: apiKey, model: model)

            guard child.isRunning, process === child else {
                throw launcherError("Proxy process exited during validation.")
            }

            cachedToken = cleanToken
            cachedUseAPI = cleanUseAPI
            localAPIKey = apiKey
            workBuddyConfig = try WorkBuddyConfig.render(apiKey: apiKey, port: Self.port, model: model)
            activeModel = model
            status = .running
            statusMessage = "Proxy started successfully. HKUST \(model.displayName) passed the live validation request."
            return true
        } catch {
            let detail = usefulFailureDetail(error)
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
        let configData = try JSONSerialization.data(withJSONObject: ["keys": [apiKey]])
        guard let configJSON = String(data: configData, encoding: .utf8) else {
            throw launcherError("Unable to prepare local proxy configuration.")
        }

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

    private func verifyHKUST(apiKey: String, model: HKUSTModel) async throws {
        guard let url = URL(string: "\(proxyBaseURL)/v1/chat/completions") else {
            throw launcherError("Invalid local proxy URL.")
        }
        let payload: [String: Any] = [
            "model": model.workBuddyID,
            "messages": [[
                "role": "user",
                "content": "Reply exactly OK."
            ]],
            "stream": false,
            "max_tokens": 8
        ]
        let body = try JSONSerialization.data(withJSONObject: payload)
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.httpBody = body
        request.timeoutInterval = 120
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.setValue("Bearer \(apiKey)", forHTTPHeaderField: "Authorization")

        let (data, response) = try await URLSession.shared.data(for: request)
        guard let http = response as? HTTPURLResponse else {
            throw launcherError("Live validation returned a non-HTTP response.")
        }
        guard http.statusCode == 200 else {
            let upstream = String(data: data, encoding: .utf8) ?? "HTTP \(http.statusCode)"
            throw launcherError("HKUST validation failed (HTTP \(http.statusCode)): \(shorten(upstream))")
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

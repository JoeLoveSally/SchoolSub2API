import Foundation

enum HKUSTModel: String, CaseIterable, Identifiable, Hashable {
    case glm52
    case deepSeekFlash
    case deepSeekPro
    case kimiK3

    static let defaultModel: HKUSTModel = .glm52

    var id: String { rawValue }

    var displayName: String {
        switch self {
        case .deepSeekFlash:
            return "DeepSeek V4 Flash"
        case .glm52:
            return "GLM-5.2"
        case .deepSeekPro:
            return "DeepSeek V4 Pro"
        case .kimiK3:
            return "Kimi K3"
        }
    }

    var upstreamID: String {
        switch self {
        case .deepSeekFlash:
            return "DeepSeek-V4-Flash-conv"
        case .glm52:
            return "GLM-5.2"
        case .deepSeekPro:
            return "DeepSeek-V4-Pro-conv"
        case .kimiK3:
            return "Kimi-K3"
        }
    }

    var workBuddyID: String {
        switch self {
        case .deepSeekFlash:
            return "HKUST-DeepSeek-V4-Flash"
        case .glm52:
            return "HKUST-GLM-5.2"
        case .deepSeekPro:
            return "HKUST-DeepSeek-V4-Pro"
        case .kimiK3:
            return "HKUST-Kimi-K3"
        }
    }

    var vendor: String {
        "OpenAI"
    }

    var maxInputTokens: Int {
        switch self {
        case .deepSeekFlash:
            return 262_144
        case .glm52:
            return 220_000
        case .deepSeekPro:
            return 65_535
        case .kimiK3:
            // Keep the WorkBuddy budget conservative even though the HKUST web-chat
            // endpoint accepted substantially larger probe requests in our tests.
            return 262_144
        }
    }

    var pickerLabel: String {
        switch self {
        case .deepSeekFlash:
            return "DeepSeek V4 Flash · 262K"
        case .glm52:
            return "GLM-5.2 · 220K · 默认"
        case .deepSeekPro:
            return "DeepSeek V4 Pro · 65K"
        case .kimiK3:
            return "Kimi K3 · 262K · 实验性"
        }
    }

    var detail: String {
        switch self {
        case .deepSeekFlash:
            return "已实测 context 上限 262,144；当前连通性以实时检测结果为准。"
        case .glm52:
            return "默认模型；已实测 context 上限 220,000，当前连通性以实时检测结果为准。"
        case .deepSeekPro:
            return "已实测 context 上限 65,535；长 Coding Agent 会话不推荐。"
        case .kimiK3:
            return "HKUST WebSocket 路径已实测；网页 UI 未公开，按 262K 保守配置。"
        }
    }

    static func from(upstreamID: String) -> HKUSTModel? {
        allCases.first { $0.upstreamID.caseInsensitiveCompare(upstreamID) == .orderedSame }
    }
}

enum LocalProxyConfig {
    // These aliases select DS2API's existing compatibility schema. The real HKUST
    // upstream model is controlled independently by HKUST_MODEL.
    static let compatibilityAliases: [String: String] = [
        HKUSTModel.glm52.workBuddyID: "deepseek-v4-flash",
        HKUSTModel.deepSeekFlash.workBuddyID: "deepseek-v4-flash",
        HKUSTModel.deepSeekPro.workBuddyID: "deepseek-v4-pro",
        HKUSTModel.kimiK3.workBuddyID: "deepseek-v4-flash"
    ]

    static func render(apiKey: String) throws -> String {
        let root: [String: Any] = [
            "keys": [apiKey],
            "model_aliases": compatibilityAliases
        ]
        let data = try JSONSerialization.data(withJSONObject: root)
        guard let text = String(data: data, encoding: .utf8) else {
            throw NSError(
                domain: "JoeJoeProxy.LocalProxyConfig",
                code: 1,
                userInfo: [NSLocalizedDescriptionKey: "Unable to prepare local proxy configuration."]
            )
        }
        return text
    }
}

enum WorkBuddyConfig {
    static let maxOutputTokens = 4_096
    static let configPath = "~/.codebuddy/models.json"

    static func render(apiKey: String, port: Int, model selectedModel: HKUSTModel) throws -> String {
        let model: [String: Any] = [
            "id": selectedModel.workBuddyID,
            "name": "HKUST \(selectedModel.displayName)",
            "vendor": selectedModel.vendor,
            "apiKey": apiKey,
            "maxInputTokens": selectedModel.maxInputTokens,
            "maxOutputTokens": maxOutputTokens,
            "url": "http://127.0.0.1:\(port)/v1/chat/completions",
            "supportsToolCall": true,
            "supportsImages": false,
            "supportsReasoning": true,
            "relatedModels": [
                "lite": selectedModel.workBuddyID,
                "reasoning": selectedModel.workBuddyID
            ]
        ]
        let root: [String: Any] = [
            "models": [model],
            "availableModels": [selectedModel.workBuddyID]
        ]
        let data = try JSONSerialization.data(
            withJSONObject: root,
            options: [.prettyPrinted, .sortedKeys]
        )
        guard let text = String(data: data, encoding: .utf8) else {
            throw NSError(
                domain: "SchoolSub2API.WorkBuddyConfig",
                code: -1,
                userInfo: [NSLocalizedDescriptionKey: "Unable to encode WorkBuddy config."]
            )
        }
        return text
    }
}

import Foundation

enum HKUSTModel: String, CaseIterable, Identifiable {
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
            return "deepseek-v4-flash"
        case .glm52:
            return "glm-5.2"
        case .deepSeekPro:
            return "deepseek-v4-pro"
        case .kimiK3:
            return "kimi-k3"
        }
    }

    var vendor: String {
        switch self {
        case .deepSeekFlash, .deepSeekPro:
            return "DeepSeek"
        case .glm52:
            return "Zhipu AI"
        case .kimiK3:
            return "Moonshot AI"
        }
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
            return "DeepSeek V4 Flash · 262K · 暂不可用"
        case .glm52:
            return "GLM-5.2 · 220K · 推荐"
        case .deepSeekPro:
            return "DeepSeek V4 Pro · 65K"
        case .kimiK3:
            return "Kimi K3 · 262K · 实验性"
        }
    }

    var detail: String {
        switch self {
        case .deepSeekFlash:
            return "学校侧当前异常；暂不作为默认，恢复后仍可手动验证。"
        case .glm52:
            return "当前默认；已实测 220,000 context，长上下文请求通常比 Flash 慢。"
        case .deepSeekPro:
            return "已实测 65,535 context；长 Coding Agent 会话不推荐。"
        case .kimiK3:
            return "HKUST WebSocket 已实测可用，但网页 UI 未公开；按 262K 保守配置。"
        }
    }
}

enum LocalProxyConfig {
    // These aliases only select DS2API's existing compatibility schema. The actual
    // HKUST upstream model is controlled independently by HKUST_MODEL.
    static let compatibilityAliases: [String: String] = [
        HKUSTModel.glm52.workBuddyID: "deepseek-v4-flash",
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

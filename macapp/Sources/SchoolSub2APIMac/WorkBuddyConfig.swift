import Foundation

enum WorkBuddyConfig {
    static let modelID = "deepseek-v4-flash"
    static let modelName = "HKUST DeepSeek V4 Flash"
    static let maxInputTokens = 262_144
    static let maxOutputTokens = 4_096
    static let configPath = "~/.codebuddy/models.json"

    static func render(apiKey: String, port: Int) throws -> String {
        let model: [String: Any] = [
            "id": modelID,
            "name": modelName,
            "vendor": "DeepSeek",
            "apiKey": apiKey,
            "maxInputTokens": maxInputTokens,
            "maxOutputTokens": maxOutputTokens,
            "url": "http://127.0.0.1:\(port)/v1/chat/completions",
            "supportsToolCall": true,
            "supportsImages": false,
            "supportsReasoning": true,
            "relatedModels": [
                "lite": modelID,
                "reasoning": modelID
            ]
        ]
        let root: [String: Any] = [
            "models": [model],
            "availableModels": [modelID]
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

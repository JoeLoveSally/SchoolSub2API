import Foundation
import XCTest
@testable import SchoolSub2APIMac

final class WorkBuddyConfigTests: XCTestCase {
    func testDefaultModelIsGLM52() {
        XCTAssertEqual(HKUSTModel.defaultModel, .glm52)
        XCTAssertEqual(HKUSTModel.allCases.first, .glm52)
    }

    func testAllModelsRenderTheirOwnVariants() throws {
        for model in HKUSTModel.allCases {
            let rendered = try WorkBuddyConfig.render(
                apiKey: "sk-local-test",
                port: 5001,
                model: model
            )
            let data = try XCTUnwrap(rendered.data(using: .utf8))
            let root = try XCTUnwrap(JSONSerialization.jsonObject(with: data) as? [String: Any])
            let models = try XCTUnwrap(root["models"] as? [[String: Any]])
            XCTAssertEqual(models.count, 1)

            let renderedModel = models[0]
            XCTAssertEqual(renderedModel["id"] as? String, model.workBuddyID)
            XCTAssertEqual(renderedModel["vendor"] as? String, "OpenAI")
            XCTAssertEqual(renderedModel["apiKey"] as? String, "sk-local-test")
            XCTAssertEqual(renderedModel["maxInputTokens"] as? Int, model.maxInputTokens)
            XCTAssertEqual(renderedModel["url"] as? String, "http://127.0.0.1:5001/v1/chat/completions")

            let related = try XCTUnwrap(renderedModel["relatedModels"] as? [String: String])
            XCTAssertEqual(related["lite"], model.workBuddyID)
            XCTAssertEqual(related["reasoning"], model.workBuddyID)

            let available = try XCTUnwrap(root["availableModels"] as? [String])
            XCTAssertEqual(available, [model.workBuddyID])
        }
    }

    func testWorkBuddyIDsAreUniqueAndUseUppercaseHKUSTPrefix() {
        let ids = HKUSTModel.allCases.map(\.workBuddyID)
        XCTAssertEqual(Set(ids).count, HKUSTModel.allCases.count)
        for id in ids {
            XCTAssertTrue(id.hasPrefix("HKUST-"), "unexpected WorkBuddy ID: \(id)")
        }
        XCTAssertEqual(HKUSTModel.glm52.workBuddyID, "HKUST-GLM-5.2")
        XCTAssertEqual(HKUSTModel.deepSeekFlash.workBuddyID, "HKUST-DeepSeek-V4-Flash")
        XCTAssertEqual(HKUSTModel.deepSeekPro.workBuddyID, "HKUST-DeepSeek-V4-Pro")
        XCTAssertEqual(HKUSTModel.kimiK3.workBuddyID, "HKUST-Kimi-K3")
    }

    func testVerifiedContextBudgets() {
        XCTAssertEqual(HKUSTModel.deepSeekFlash.maxInputTokens, 262_144)
        XCTAssertEqual(HKUSTModel.glm52.maxInputTokens, 220_000)
        XCTAssertEqual(HKUSTModel.deepSeekPro.maxInputTokens, 65_535)
        XCTAssertEqual(HKUSTModel.kimiK3.maxInputTokens, 262_144)
    }

    func testUpstreamModelIDs() {
        XCTAssertEqual(HKUSTModel.deepSeekFlash.upstreamID, "DeepSeek-V4-Flash-conv")
        XCTAssertEqual(HKUSTModel.glm52.upstreamID, "GLM-5.2")
        XCTAssertEqual(HKUSTModel.deepSeekPro.upstreamID, "DeepSeek-V4-Pro-conv")
        XCTAssertEqual(HKUSTModel.kimiK3.upstreamID, "Kimi-K3")
        XCTAssertEqual(HKUSTModel.from(upstreamID: "kimi-k3"), .kimiK3)
    }

    func testLocalProxyConfigAcceptsAllHKUSTWorkBuddyIDs() throws {
        let rendered = try LocalProxyConfig.render(apiKey: "sk-local-test")
        let data = try XCTUnwrap(rendered.data(using: .utf8))
        let root = try XCTUnwrap(JSONSerialization.jsonObject(with: data) as? [String: Any])
        let aliases = try XCTUnwrap(root["model_aliases"] as? [String: String])

        XCTAssertEqual(aliases[HKUSTModel.glm52.workBuddyID], "deepseek-v4-flash")
        XCTAssertEqual(aliases[HKUSTModel.deepSeekFlash.workBuddyID], "deepseek-v4-flash")
        XCTAssertEqual(aliases[HKUSTModel.deepSeekPro.workBuddyID], "deepseek-v4-pro")
        XCTAssertEqual(aliases[HKUSTModel.kimiK3.workBuddyID], "deepseek-v4-flash")
        XCTAssertEqual(root["keys"] as? [String], ["sk-local-test"])
        XCTAssertNil(root["token"])
        XCTAssertNil(root["useApi"])
    }

    func testLocalAPIKeyStoreUsesPrivateFileInsteadOfKeychain() throws {
        let fileManager = FileManager.default
        let base = fileManager.temporaryDirectory
            .appendingPathComponent("JoeJoeProxyTests-\(UUID().uuidString)", isDirectory: true)
        defer { try? fileManager.removeItem(at: base) }

        let first = try LocalAPIKeyStore.loadOrCreate(applicationSupportDirectory: base)
        let second = try LocalAPIKeyStore.loadOrCreate(applicationSupportDirectory: base)
        XCTAssertEqual(first, second)
        XCTAssertTrue(first.hasPrefix("sk-local-"))

        let directory = base.appendingPathComponent("JoeJoeProxy", isDirectory: true)
        let keyFile = directory.appendingPathComponent("local_api_key", isDirectory: false)
        XCTAssertEqual(try String(contentsOf: keyFile, encoding: .utf8), first)

        let directoryAttributes = try fileManager.attributesOfItem(atPath: directory.path)
        let fileAttributes = try fileManager.attributesOfItem(atPath: keyFile.path)
        XCTAssertEqual((directoryAttributes[.posixPermissions] as? NSNumber)?.intValue, 0o700)
        XCTAssertEqual((fileAttributes[.posixPermissions] as? NSNumber)?.intValue, 0o600)
    }
}

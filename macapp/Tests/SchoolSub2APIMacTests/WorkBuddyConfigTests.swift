import XCTest
@testable import SchoolSub2APIMac

final class WorkBuddyConfigTests: XCTestCase {
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
    }
}

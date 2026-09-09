import XCTest
@testable import SchoolSub2APIMac

final class WorkBuddyConfigTests: XCTestCase {
    func testRenderUsesFlashForAllVariants() throws {
        let rendered = try WorkBuddyConfig.render(apiKey: "sk-local-test", port: 5001)
        let data = try XCTUnwrap(rendered.data(using: .utf8))
        let root = try XCTUnwrap(JSONSerialization.jsonObject(with: data) as? [String: Any])
        let models = try XCTUnwrap(root["models"] as? [[String: Any]])
        XCTAssertEqual(models.count, 1)

        let model = models[0]
        XCTAssertEqual(model["id"] as? String, "deepseek-v4-flash")
        XCTAssertEqual(model["apiKey"] as? String, "sk-local-test")
        XCTAssertEqual(model["maxInputTokens"] as? Int, 262_144)
        XCTAssertEqual(model["url"] as? String, "http://127.0.0.1:5001/v1/chat/completions")

        let related = try XCTUnwrap(model["relatedModels"] as? [String: String])
        XCTAssertEqual(related["lite"], "deepseek-v4-flash")
        XCTAssertEqual(related["reasoning"], "deepseek-v4-flash")
    }

    func testAvailableModelsOnlyContainsFlash() throws {
        let rendered = try WorkBuddyConfig.render(apiKey: "sk-local-test", port: 5001)
        let data = try XCTUnwrap(rendered.data(using: .utf8))
        let root = try XCTUnwrap(JSONSerialization.jsonObject(with: data) as? [String: Any])
        let available = try XCTUnwrap(root["availableModels"] as? [String])
        XCTAssertEqual(available, ["deepseek-v4-flash"])
    }
}

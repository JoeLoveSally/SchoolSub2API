import Foundation

enum LocalAPIKeyStore {
    private static let directoryName = "JoeJoeProxy"
    private static let fileName = "local_api_key"

    static func loadOrCreate(applicationSupportDirectory: URL? = nil) throws -> String {
        let fileURL = try keyURL(applicationSupportDirectory: applicationSupportDirectory)
        if let existing = try read(from: fileURL), !existing.isEmpty {
            return existing
        }

        let created = generate()
        try save(created, to: fileURL)
        return created
    }

    private static func keyURL(applicationSupportDirectory: URL?) throws -> URL {
        let base: URL
        if let applicationSupportDirectory {
            base = applicationSupportDirectory
        } else {
            guard let resolved = FileManager.default.urls(
                for: .applicationSupportDirectory,
                in: .userDomainMask
            ).first else {
                throw storeError("Unable to locate Application Support directory.")
            }
            base = resolved
        }

        let directory = base.appendingPathComponent(directoryName, isDirectory: true)
        try FileManager.default.createDirectory(
            at: directory,
            withIntermediateDirectories: true,
            attributes: [.posixPermissions: 0o700]
        )
        try FileManager.default.setAttributes(
            [.posixPermissions: 0o700],
            ofItemAtPath: directory.path
        )
        return directory.appendingPathComponent(fileName, isDirectory: false)
    }

    private static func read(from fileURL: URL) throws -> String? {
        let fileManager = FileManager.default
        guard fileManager.fileExists(atPath: fileURL.path) else {
            return nil
        }

        try fileManager.setAttributes(
            [.posixPermissions: 0o600],
            ofItemAtPath: fileURL.path
        )
        let data = try Data(contentsOf: fileURL)
        guard let value = String(data: data, encoding: .utf8) else {
            throw storeError("Unable to decode the local API key file.")
        }
        let cleanValue = value.trimmingCharacters(in: .whitespacesAndNewlines)
        return cleanValue.isEmpty ? nil : cleanValue
    }

    private static func save(_ value: String, to fileURL: URL) throws {
        let fileManager = FileManager.default
        if fileManager.fileExists(atPath: fileURL.path) {
            try fileManager.removeItem(at: fileURL)
        }

        let created = fileManager.createFile(
            atPath: fileURL.path,
            contents: Data(value.utf8),
            attributes: [.posixPermissions: 0o600]
        )
        guard created else {
            throw storeError("Unable to create the local API key file.")
        }
        try fileManager.setAttributes(
            [.posixPermissions: 0o600],
            ofItemAtPath: fileURL.path
        )
    }

    private static func generate() -> String {
        var generator = SystemRandomNumberGenerator()
        let bytes = (0..<32).map { _ in
            UInt8.random(in: UInt8.min...UInt8.max, using: &generator)
        }
        let token = Data(bytes)
            .base64EncodedString()
            .replacingOccurrences(of: "+", with: "-")
            .replacingOccurrences(of: "/", with: "_")
            .replacingOccurrences(of: "=", with: "")
        return "sk-local-\(token)"
    }

    private static func storeError(_ message: String) -> NSError {
        NSError(
            domain: "JoeJoeProxy.LocalAPIKeyStore",
            code: 1,
            userInfo: [NSLocalizedDescriptionKey: message]
        )
    }
}

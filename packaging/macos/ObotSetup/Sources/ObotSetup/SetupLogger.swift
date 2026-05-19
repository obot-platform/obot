import Foundation

protocol SetupLogging: Sendable {
    func resetRun(appVersion: String)
    func info(_ message: String)
    func error(_ message: String)
}

struct NoopSetupLogger: SetupLogging {
    func resetRun(appVersion: String) {}
    func info(_ message: String) {}
    func error(_ message: String) {}
}

final class LocalSetupLogger: SetupLogging, @unchecked Sendable {
    private let fileURL: URL
    private let fileManager: FileManager
    private let lock = NSLock()
    private let timestampFormatter = ISO8601DateFormatter()
    private var disabled = false

    init(
        fileURL: URL = LocalSetupLogger.defaultLogFileURL(),
        fileManager: FileManager = .default
    ) {
        self.fileURL = fileURL
        self.fileManager = fileManager
        timestampFormatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
    }

    static func defaultLogFileURL() -> URL {
        FileManager.default.homeDirectoryForCurrentUser
            .appendingPathComponent("Library", isDirectory: true)
            .appendingPathComponent("Logs", isDirectory: true)
            .appendingPathComponent("Obot Setup", isDirectory: true)
            .appendingPathComponent("setup.log", isDirectory: false)
    }

    func resetRun(appVersion: String) {
        lock.lock()
        defer { lock.unlock() }

        disabled = false
        do {
            try fileManager.createDirectory(
                at: fileURL.deletingLastPathComponent(),
                withIntermediateDirectories: true
            )

            let header = [
                line(level: "INFO", message: "app_started appVersion=\(SetupLogSanitizer.field(appVersion))"),
                line(level: "INFO", message: "log_policy minimal=true secrets_redacted=true overwrite_per_run=true"),
            ].joined(separator: "\n") + "\n"
            try header.write(to: fileURL, atomically: true, encoding: .utf8)
        } catch {
            disabled = true
        }
    }

    func info(_ message: String) {
        append(level: "INFO", message: message)
    }

    func error(_ message: String) {
        append(level: "ERROR", message: message)
    }

    private func append(level: String, message: String) {
        lock.lock()
        defer { lock.unlock() }

        guard !disabled else {
            return
        }

        do {
            if !fileManager.fileExists(atPath: fileURL.path) {
                try fileManager.createDirectory(
                    at: fileURL.deletingLastPathComponent(),
                    withIntermediateDirectories: true
                )
                try Data().write(to: fileURL, options: .atomic)
            }

            let handle = try FileHandle(forWritingTo: fileURL)
            defer { try? handle.close() }
            try handle.seekToEnd()
            if let data = (line(level: level, message: message) + "\n").data(using: .utf8) {
                try handle.write(contentsOf: data)
            }
        } catch {
            disabled = true
        }
    }

    private func line(level: String, message: String) -> String {
        "\(timestampFormatter.string(from: Date())) \(level) \(SetupLogSanitizer.message(message))"
    }
}

enum SetupLogSanitizer {
    static func field(_ value: String) -> String {
        let sanitized = message(value)
            .replacingOccurrences(of: "\n", with: " ")
            .replacingOccurrences(of: "\r", with: " ")
        return sanitized.isEmpty ? "<empty>" : sanitized
    }

    static func message(_ value: String) -> String {
        var sanitized = value
        sanitized = replaceMatches(
            in: sanitized,
            pattern: #"https?://[^\s"']+"#,
            options: [.caseInsensitive]
        ) { match in
            urlOrigin(match) ?? "<redacted-url>"
        }
        sanitized = replaceMatches(
            in: sanitized,
            pattern: #"(?i)(bearer)\s+[A-Za-z0-9._~+/=-]+"#
        ) { match in
            let prefix = match.split(separator: " ", maxSplits: 1).first ?? "Bearer"
            return "\(prefix) <redacted>"
        }
        sanitized = replaceMatches(
            in: sanitized,
            pattern: #"(?i)\b(token|access_token|refresh_token|id_token|password|secret|authorization)=([^\s&]+)"#
        ) { match in
            guard let equals = match.firstIndex(of: "=") else {
                return "<redacted>"
            }
            return "\(match[..<equals])=<redacted>"
        }
        sanitized = replaceMatches(
            in: sanitized,
            pattern: #"/Users/[^/\s"']+(/[^\s"']*)?"#
        ) { _ in
            "~/<redacted-path>"
        }
        sanitized = replaceMatches(
            in: sanitized,
            pattern: #"/private/var/folders/[^\s"']+"#
        ) { _ in
            "<redacted-temp-path>"
        }
        return sanitized
    }

    static func urlOrigin(_ raw: String) -> String? {
        var trimmed = raw
        var trailing = ""
        while let last = trimmed.last, [".", ",", ";", ":", ")", "]"].contains(last) {
            trailing.insert(last, at: trailing.startIndex)
            trimmed.removeLast()
        }

        guard let components = URLComponents(string: trimmed),
              let scheme = components.scheme?.lowercased(),
              (scheme == "http" || scheme == "https"),
              let host = components.host,
              !host.isEmpty
        else {
            return nil
        }

        var origin = "\(scheme)://\(host)"
        if let port = components.port {
            origin += ":\(port)"
        }
        return origin + trailing
    }

    private static func replaceMatches(
        in value: String,
        pattern: String,
        options: NSRegularExpression.Options = [],
        replacement: (String) -> String
    ) -> String {
        guard let regex = try? NSRegularExpression(pattern: pattern, options: options) else {
            return value
        }

        let nsValue = value as NSString
        let matches = regex.matches(
            in: value,
            options: [],
            range: NSRange(location: 0, length: nsValue.length)
        )

        guard !matches.isEmpty else {
            return value
        }

        var result = value
        for match in matches.reversed() {
            let matched = nsValue.substring(with: match.range)
            guard let range = Range(match.range, in: result) else {
                continue
            }
            result.replaceSubrange(range, with: replacement(matched))
        }
        return result
    }
}

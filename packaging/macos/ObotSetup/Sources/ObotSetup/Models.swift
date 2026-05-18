import Foundation

let obotCLIPath = "/usr/local/bin/obot"

struct SetupStatus: Decodable, Equatable {
    var version: String
    var capabilities: [String]
    var defaultURL: String
    var tokenValid: Bool
    var setupComplete: Bool

    var capabilitySet: Set<String> {
        Set(capabilities)
    }
}

struct AgentDetectionOutput: Decodable, Equatable {
    var agents: [DetectedAgent]
}

struct DetectedAgent: Decodable, Equatable, Identifiable {
    var id: String
    var displayName: String
    var state: String
    var reason: String

    var isPresent: Bool {
        state == "present"
    }
}

enum SetupDisplayState: Equatable {
    case loading
    case missingCLI
    case unsupportedCLI(version: String?, missingCapabilities: [String], message: String?)
    case firstRun(status: SetupStatus)
    case alreadyConfigured(status: SetupStatus)
    case needsLoginRepair(status: SetupStatus)
}

struct SetupStateResolver {
    static let requiredCapabilities = [
        "setup.nonInteractive",
        "setup.detectAgents",
        "setup.progressJSON",
    ]

    static func resolve(cliExists: Bool, status: SetupStatus?, statusError: String? = nil) -> SetupDisplayState {
        guard cliExists else {
            return .missingCLI
        }

        guard let status else {
            return .unsupportedCLI(
                version: nil,
                missingCapabilities: requiredCapabilities,
                message: statusError
            )
        }

        let missingCapabilities = requiredCapabilities.filter { !status.capabilitySet.contains($0) }
        if !missingCapabilities.isEmpty {
            return .unsupportedCLI(
                version: status.version,
                missingCapabilities: missingCapabilities,
                message: nil
            )
        }

        if status.defaultURL.isEmpty {
            return .firstRun(status: status)
        }

        if status.setupComplete {
            return .alreadyConfigured(status: status)
        }

        return .needsLoginRepair(status: status)
    }
}

enum SetupWarning: Equatable, Identifiable {
    case pathMissing(path: String)
    case versionMismatch(cliVersion: String, appVersion: String)

    var id: String {
        switch self {
        case .pathMissing(let path):
            return "pathMissing:\(path)"
        case .versionMismatch(let cliVersion, let appVersion):
            return "versionMismatch:\(cliVersion):\(appVersion)"
        }
    }

    var title: String {
        switch self {
        case .pathMissing:
            return "/usr/local/bin is not on PATH"
        case .versionMismatch:
            return "CLI version differs from setup app"
        }
    }

    var message: String {
        switch self {
        case .pathMissing(let path):
            return "A new terminal may not find obot until /usr/local/bin is added to PATH. Current PATH: \(path)"
        case .versionMismatch(let cliVersion, let appVersion):
            return "Installed CLI version \(cliVersion) does not match setup app version \(appVersion)."
        }
    }
}

struct SetupWarningResolver {
    static func warnings(status: SetupStatus?, appVersion: String, environmentPath: String?) -> [SetupWarning] {
        var warnings: [SetupWarning] = []

        let path = environmentPath ?? ""
        let pathEntries = path.split(separator: ":").map(String.init)
        if !pathEntries.contains("/usr/local/bin") {
            warnings.append(.pathMissing(path: path.isEmpty ? "(empty)" : path))
        }

        if let cliVersion = status?.version, !appVersion.isEmpty, cliVersion != appVersion {
            warnings.append(.versionMismatch(cliVersion: cliVersion, appVersion: appVersion))
        }

        return warnings
    }
}

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

struct SetupRunRequest: Equatable, Sendable {
    var url: String
    var selectedAgentIDs: [String]

    var agentArgument: String {
        if selectedAgentIDs.isEmpty {
            return "none"
        }
        return selectedAgentIDs.joined(separator: ",")
    }

    var arguments: [String] {
        [
            "setup",
            "--url", url,
            "--agents", agentArgument,
            "--yes",
            "--non-interactive",
            "--output", "json",
        ]
    }
}

struct SetupProgressEvent: Decodable, Equatable, Identifiable, Sendable {
    var type: String
    var code: String?
    var message: String?
    var url: String?
    var agentID: String?
    var displayName: String?
    var installed: [String]?

    var id: String {
        [
            type,
            code ?? "",
            message ?? "",
            url ?? "",
            agentID ?? "",
            displayName ?? "",
        ].joined(separator: ":")
    }
}

struct SetupErrorDisplay: Equatable, Sendable {
    var code: String
    var title: String
    var message: String

    static func from(event: SetupProgressEvent) -> SetupErrorDisplay {
        let code = event.code ?? "unknown"
        return SetupErrorDisplay(
            code: code,
            title: title(for: code),
            message: event.message ?? "Setup failed."
        )
    }

    static func unknown(_ message: String) -> SetupErrorDisplay {
        SetupErrorDisplay(code: "unknown", title: title(for: "unknown"), message: message)
    }

    static func canceled() -> SetupErrorDisplay {
        SetupErrorDisplay(code: "auth_canceled", title: title(for: "auth_canceled"), message: "Setup was canceled.")
    }

    private static func title(for code: String) -> String {
        switch code {
        case "invalid_url":
            return "Check the Obot URL"
        case "server_unreachable":
            return "Obot server is unreachable"
        case "auth_unavailable":
            return "Authentication is unavailable"
        case "auth_timeout":
            return "Authentication timed out"
        case "auth_canceled":
            return "Setup was canceled"
        case "config_save_failed":
            return "Could not save configuration"
        case "agent_detection_failed":
            return "Could not detect local agents"
        case "agent_install_failed":
            return "Could not install local agent support"
        case "unsupported_cli":
            return "Installed CLI is not supported"
        default:
            return "Setup failed"
        }
    }
}

struct SetupProgressState: Equatable, Sendable {
    var events: [SetupProgressEvent] = []
    var error: SetupErrorDisplay?
    var isComplete = false

    mutating func apply(_ event: SetupProgressEvent) {
        events.append(event)
        switch event.type {
        case "complete":
            isComplete = true
        case "error":
            error = SetupErrorDisplay.from(event: event)
        default:
            break
        }
    }
}

enum SetupFlowScreen: Equatable {
    case status
    case url
    case agents
    case running
    case done
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

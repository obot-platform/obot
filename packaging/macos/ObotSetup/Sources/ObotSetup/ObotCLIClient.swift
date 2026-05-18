import Foundation

struct ObotCLIClient: Sendable {
    var executableURL: URL = URL(fileURLWithPath: obotCLIPath)
    var runner: any CommandRunning = ProcessCommandRunner()

    func cliExists() -> Bool {
        FileManager.default.isExecutableFile(atPath: executableURL.path)
    }

    func loadStatus() throws -> SetupStatus {
        let data = try runner.run(executableURL: executableURL, arguments: ["setup", "status", "--json"])
        return try JSONDecoder().decode(SetupStatus.self, from: data)
    }

    func loadAgents() throws -> AgentDetectionOutput {
        let data = try runner.run(executableURL: executableURL, arguments: ["setup", "detect-agents", "--json"])
        return try JSONDecoder().decode(AgentDetectionOutput.self, from: data)
    }
}

protocol CommandRunning: Sendable {
    func run(executableURL: URL, arguments: [String]) throws -> Data
}

struct ProcessCommandRunner: CommandRunning {
    func run(executableURL: URL, arguments: [String]) throws -> Data {
        let process = Process()
        let stdout = Pipe()
        let stderr = Pipe()

        process.executableURL = executableURL
        process.arguments = arguments
        process.standardOutput = stdout
        process.standardError = stderr

        do {
            try process.run()
        } catch {
            throw CLIInvocationError.launchFailed(error.localizedDescription)
        }

        process.waitUntilExit()

        let output = stdout.fileHandleForReading.readDataToEndOfFile()
        let errorOutput = stderr.fileHandleForReading.readDataToEndOfFile()

        guard process.terminationStatus == 0 else {
            let message = String(data: errorOutput, encoding: .utf8)?
                .trimmingCharacters(in: .whitespacesAndNewlines)
            throw CLIInvocationError.commandFailed(
                exitCode: process.terminationStatus,
                message: message?.isEmpty == false ? message! : "obot exited with status \(process.terminationStatus)"
            )
        }

        return output
    }
}

enum CLIInvocationError: Error, LocalizedError, Equatable {
    case launchFailed(String)
    case commandFailed(exitCode: Int32, message: String)

    var errorDescription: String? {
        switch self {
        case .launchFailed(let message):
            return "Could not launch obot: \(message)"
        case .commandFailed(_, let message):
            return message
        }
    }
}

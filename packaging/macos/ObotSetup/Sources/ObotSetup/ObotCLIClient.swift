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

protocol SetupCommandRunning: Sendable {
    func runSetup(
        executableURL: URL,
        request: SetupRunRequest,
        onEvent: @escaping @Sendable (SetupProgressEvent) -> Void
    ) async -> SetupRunResult

    func cancel()
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

struct SetupRunResult: Sendable {
    var exitCode: Int32?
    var events: [SetupProgressEvent]
    var stderr: String
    var canceled: Bool
    var launchError: String?
    var parseErrors: [String]

    var errorDisplay: SetupErrorDisplay? {
        if canceled {
            return .canceled()
        }

        if let event = events.last(where: { $0.type == "error" }) {
            return .from(event: event)
        }

        if let launchError {
            return .unknown(launchError)
        }

        if let parseError = parseErrors.first {
            return .unknown(parseError)
        }

        if let exitCode, exitCode != 0 {
            let message = stderr.trimmingCharacters(in: .whitespacesAndNewlines)
            return .unknown(message.isEmpty ? "obot exited with status \(exitCode)" : message)
        }

        return nil
    }

    var succeeded: Bool {
        errorDisplay == nil && events.contains(where: { $0.type == "complete" })
    }
}

final class ProcessSetupRunner: SetupCommandRunning, @unchecked Sendable {
    private let lock = NSLock()
    private let logger: any SetupLogging
    private var process: Process?
    private var cancellationRequested = false

    init(logger: any SetupLogging = NoopSetupLogger()) {
        self.logger = logger
    }

    func runSetup(
        executableURL: URL,
        request: SetupRunRequest,
        onEvent: @escaping @Sendable (SetupProgressEvent) -> Void
    ) async -> SetupRunResult {
        await withCheckedContinuation { continuation in
            DispatchQueue.global(qos: .userInitiated).async {
                continuation.resume(
                    returning: self.runSetupSync(
                        executableURL: executableURL,
                        request: request,
                        onEvent: onEvent
                    )
                )
            }
        }
    }

    func cancel() {
        lock.lock()
        cancellationRequested = true
        let process = process
        lock.unlock()

        process?.terminate()
    }

    private func runSetupSync(
        executableURL: URL,
        request: SetupRunRequest,
        onEvent: @escaping @Sendable (SetupProgressEvent) -> Void
    ) -> SetupRunResult {
        let process = Process()
        let stdout = Pipe()
        let stderr = Pipe()
        let outputLock = NSLock()
        let stderrLock = NSLock()
        let group = DispatchGroup()
        var events: [SetupProgressEvent] = []
        var parseErrors: [String] = []
        var stderrData = Data()

        logger.info(
            "setup_process_start executable=\(SetupLogSanitizer.field(executableURL.path)) " +
            "url=\(SetupLogSanitizer.field(SetupLogSanitizer.urlOrigin(request.url) ?? "<invalid-url>")) " +
            "agents=\(SetupLogSanitizer.field(request.agentArgument))"
        )

        process.executableURL = executableURL
        process.arguments = request.arguments
        process.standardOutput = stdout
        process.standardError = stderr

        lock.lock()
        self.process = process
        cancellationRequested = false
        lock.unlock()

        group.enter()
        DispatchQueue.global(qos: .utility).async {
            var buffer = ""
            let decoder = JSONDecoder()

            func parseLine(_ line: String) {
                let trimmed = line.trimmingCharacters(in: .whitespacesAndNewlines)
                guard !trimmed.isEmpty else {
                    return
                }
                do {
                    let event = try decoder.decode(SetupProgressEvent.self, from: Data(trimmed.utf8))
                    outputLock.lock()
                    events.append(event)
                    outputLock.unlock()
                    self.logger.info(setupEventLogLine(event))
                    onEvent(event)
                } catch {
                    outputLock.lock()
                    parseErrors.append("Could not parse setup progress: \(error.localizedDescription)")
                    outputLock.unlock()
                    self.logger.error("setup_parse_error message=\(SetupLogSanitizer.field(error.localizedDescription))")
                }
            }

            while true {
                let data = stdout.fileHandleForReading.availableData
                if data.isEmpty {
                    break
                }
                guard let chunk = String(data: data, encoding: .utf8) else {
                    outputLock.lock()
                    parseErrors.append("Setup output was not valid UTF-8.")
                    outputLock.unlock()
                    self.logger.error("setup_output_invalid_utf8")
                    continue
                }
                buffer += chunk
                while let newline = buffer.firstIndex(of: "\n") {
                    let line = String(buffer[..<newline])
                    buffer.removeSubrange(...newline)
                    parseLine(line)
                }
            }

            parseLine(buffer)
            group.leave()
        }

        group.enter()
        DispatchQueue.global(qos: .utility).async {
            let data = stderr.fileHandleForReading.readDataToEndOfFile()
            stderrLock.lock()
            stderrData = data
            stderrLock.unlock()
            group.leave()
        }

        do {
            try process.run()
        } catch {
            stdout.fileHandleForReading.closeFile()
            stderr.fileHandleForReading.closeFile()
            group.wait()
            clearProcess(process)
            logger.error("setup_launch_failed message=\(SetupLogSanitizer.field(error.localizedDescription))")
            return SetupRunResult(
                exitCode: nil,
                events: events,
                stderr: "",
                canceled: wasCanceled(),
                launchError: "Could not launch obot: \(error.localizedDescription)",
                parseErrors: parseErrors
            )
        }

        process.waitUntilExit()
        group.wait()

        outputLock.lock()
        let parsedEvents = events
        let parsedErrors = parseErrors
        outputLock.unlock()

        stderrLock.lock()
        let stderrText = String(data: stderrData, encoding: .utf8) ?? ""
        stderrLock.unlock()

        let canceled = wasCanceled()
        clearProcess(process)
        logger.info(
            "setup_process_exit exitCode=\(process.terminationStatus) canceled=\(canceled) " +
            "events=\(parsedEvents.count) parseErrors=\(parsedErrors.count) stderrPresent=\(!stderrText.isEmpty)"
        )

        return SetupRunResult(
            exitCode: process.terminationStatus,
            events: parsedEvents,
            stderr: stderrText,
            canceled: canceled,
            launchError: nil,
            parseErrors: parsedErrors
        )
    }

    private func clearProcess(_ process: Process) {
        lock.lock()
        if self.process === process {
            self.process = nil
        }
        lock.unlock()
    }

    private func wasCanceled() -> Bool {
        lock.lock()
        let canceled = cancellationRequested
        lock.unlock()
        return canceled
    }
}

private func setupEventLogLine(_ event: SetupProgressEvent) -> String {
    var fields = ["setup_event type=\(SetupLogSanitizer.field(event.type))"]
    if let code = event.code {
        fields.append("code=\(SetupLogSanitizer.field(code))")
    }
    if let url = event.url {
        fields.append("url=\(SetupLogSanitizer.field(SetupLogSanitizer.urlOrigin(url) ?? "<invalid-url>"))")
    }
    if let agentID = event.agentID {
        fields.append("agentID=\(SetupLogSanitizer.field(agentID))")
    }
    if let displayName = event.displayName {
        fields.append("displayName=\(SetupLogSanitizer.field(displayName))")
    }
    if let installed = event.installed {
        fields.append("installedCount=\(installed.count)")
    }
    if let message = event.message {
        fields.append("message=\(SetupLogSanitizer.field(message))")
    }
    return fields.joined(separator: " ")
}

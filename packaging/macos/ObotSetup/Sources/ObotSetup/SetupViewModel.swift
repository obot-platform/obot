import Foundation

@MainActor
final class SetupViewModel: ObservableObject {
    @Published private(set) var displayState: SetupDisplayState = .loading
    @Published private(set) var agents: [DetectedAgent] = []
    @Published private(set) var warnings: [SetupWarning] = []
    @Published private(set) var screen: SetupFlowScreen = .status
    @Published var urlText: String = ""
    @Published private(set) var selectedAgentIDs: Set<String> = []
    @Published private(set) var urlValidationMessage: String?
    @Published private(set) var progress = SetupProgressState()
    @Published private(set) var isRunningSetup = false

    private let cliClient: ObotCLIClient
    private let setupRunner: any SetupCommandRunning
    private let logger: any SetupLogging
    private let appVersion: String
    private let environmentPath: String?

    init(
        cliClient: ObotCLIClient = ObotCLIClient(),
        setupRunner: (any SetupCommandRunning)? = nil,
        logger: any SetupLogging = LocalSetupLogger(),
        appVersion: String = Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? "dev",
        environmentPath: String? = ProcessInfo.processInfo.environment["PATH"]
    ) {
        self.cliClient = cliClient
        self.setupRunner = setupRunner ?? ProcessSetupRunner(logger: logger)
        self.logger = logger
        self.appVersion = appVersion
        self.environmentPath = environmentPath
        logger.resetRun(appVersion: appVersion)
    }

    func load() {
        displayState = .loading
        screen = .status
        logger.info("load_start")

        let cliClient = cliClient
        let appVersion = appVersion
        let environmentPath = environmentPath
        let logger = logger

        Task {
            let snapshot = await Task.detached(priority: .userInitiated) {
                loadSetupSnapshot(
                    cliClient: cliClient,
                    appVersion: appVersion,
                    environmentPath: environmentPath,
                    logger: logger
                )
            }.value

            displayState = snapshot.displayState
            agents = snapshot.agents
            warnings = snapshot.warnings
            selectedAgentIDs = Set(snapshot.agents.filter(\.isPresent).map(\.id))
            urlText = snapshot.configuredURL
        }
    }

    func startSetupFlow() {
        urlValidationMessage = nil
        progress = SetupProgressState()
        screen = .url
        logger.info("setup_flow_start")
    }

    func confirmURL() {
        let trimmed = urlText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard isValidObotURL(trimmed) else {
            urlValidationMessage = "Enter a valid http or https URL."
            logger.info("url_validation_failed reason=invalid_format")
            return
        }

        urlText = trimmed
        urlValidationMessage = nil
        screen = .agents
        logger.info("url_confirmed url=\(SetupLogSanitizer.field(SetupLogSanitizer.urlOrigin(trimmed) ?? "<invalid-url>"))")
    }

    func backToStatus() {
        guard !isRunningSetup else {
            return
        }
        screen = .status
        logger.info("navigation screen=status")
    }

    func backToURL() {
        guard !isRunningSetup else {
            return
        }
        screen = .url
        logger.info("navigation screen=url")
    }

    func toggleAgent(_ agent: DetectedAgent) {
        guard agent.isPresent, !isRunningSetup else {
            return
        }

        if selectedAgentIDs.contains(agent.id) {
            selectedAgentIDs.remove(agent.id)
            logger.info("agent_selection_changed id=\(SetupLogSanitizer.field(agent.id)) selected=false")
        } else {
            selectedAgentIDs.insert(agent.id)
            logger.info("agent_selection_changed id=\(SetupLogSanitizer.field(agent.id)) selected=true")
        }
    }

    func runSetup() {
        guard !isRunningSetup else {
            return
        }

        let selectedIDs = agents
            .filter { $0.isPresent && selectedAgentIDs.contains($0.id) }
            .map(\.id)
            .sorted()
        let request = SetupRunRequest(url: urlText, selectedAgentIDs: selectedIDs)
        let executableURL = cliClient.executableURL
        let setupRunner = setupRunner

        progress = SetupProgressState()
        isRunningSetup = true
        screen = .running
        logger.info(
            "setup_run_start url=\(SetupLogSanitizer.field(SetupLogSanitizer.urlOrigin(request.url) ?? "<invalid-url>")) " +
            "agents=\(SetupLogSanitizer.field(request.agentArgument))"
        )

        Task {
            let result = await setupRunner.runSetup(executableURL: executableURL, request: request) { [weak self] event in
                Task { @MainActor in
                    self?.progress.apply(event)
                }
            }

            if result.succeeded {
                isRunningSetup = false
                screen = .done
                logger.info("setup_run_succeeded")
                return
            }

            if let error = result.errorDisplay {
                progress.error = error
                logger.error(
                    "setup_run_failed code=\(SetupLogSanitizer.field(error.code)) " +
                    "message=\(SetupLogSanitizer.field(error.message))"
                )
            }
            isRunningSetup = false
        }
    }

    func cancelSetup() {
        guard isRunningSetup else {
            return
        }
        logger.info("setup_cancel_requested")
        setupRunner.cancel()
    }
}

private struct SetupLoadSnapshot: Sendable {
    var displayState: SetupDisplayState
    var agents: [DetectedAgent]
    var warnings: [SetupWarning]
    var configuredURL: String
}

private func loadSetupSnapshot(
    cliClient: ObotCLIClient,
    appVersion: String,
    environmentPath: String?,
    logger: any SetupLogging
) -> SetupLoadSnapshot {
    let cliExists = cliClient.cliExists()
    guard cliExists else {
        logger.error("cli_missing expectedPath=\(SetupLogSanitizer.field(cliClient.executableURL.path))")
        return SetupLoadSnapshot(
            displayState: SetupStateResolver.resolve(cliExists: false, status: nil),
            agents: [],
            warnings: SetupWarningResolver.warnings(
                status: nil,
                appVersion: appVersion,
                environmentPath: environmentPath
            ),
            configuredURL: ""
        )
    }

    do {
        let status = try cliClient.loadStatus()
        logger.info(
            "status_loaded version=\(SetupLogSanitizer.field(status.version)) " +
            "setupComplete=\(status.setupComplete) tokenValid=\(status.tokenValid) " +
            "defaultURL=\(SetupLogSanitizer.field(SetupLogSanitizer.urlOrigin(status.defaultURL) ?? "<empty>"))"
        )
        let displayState = SetupStateResolver.resolve(cliExists: true, status: status)
        let warnings = SetupWarningResolver.warnings(
            status: status,
            appVersion: appVersion,
            environmentPath: environmentPath
        )
        logWarnings(warnings, logger: logger)

        if case .unsupportedCLI = displayState {
            logger.error("cli_unsupported reason=missing_capabilities")
            return SetupLoadSnapshot(
                displayState: displayState,
                agents: [],
                warnings: warnings,
                configuredURL: status.defaultURL
            )
        }

        let agents: [DetectedAgent]
        do {
            agents = try cliClient.loadAgents().agents
        } catch {
            logger.error("agent_detection_failed message=\(SetupLogSanitizer.field(error.localizedDescription))")
            agents = []
        }
        logger.info(
            "agents_loaded count=\(agents.count) present=\(agents.filter(\.isPresent).count) " +
            "missing=\(agents.filter { !$0.isPresent }.count)"
        )
        return SetupLoadSnapshot(
            displayState: displayState,
            agents: agents,
            warnings: warnings,
            configuredURL: status.defaultURL
        )
    } catch {
        logger.error("status_load_failed message=\(SetupLogSanitizer.field(error.localizedDescription))")
        let warnings = SetupWarningResolver.warnings(
            status: nil,
            appVersion: appVersion,
            environmentPath: environmentPath
        )
        logWarnings(warnings, logger: logger)
        return SetupLoadSnapshot(
            displayState: SetupStateResolver.resolve(
                cliExists: true,
                status: nil,
                statusError: error.localizedDescription
            ),
            agents: [],
            warnings: warnings,
            configuredURL: ""
        )
    }
}

private func isValidObotURL(_ raw: String) -> Bool {
    guard let components = URLComponents(string: raw),
          let scheme = components.scheme?.lowercased(),
          (scheme == "http" || scheme == "https"),
          let host = components.host,
          !host.isEmpty
    else {
        return false
    }

    return true
}

private func logWarnings(_ warnings: [SetupWarning], logger: any SetupLogging) {
    for warning in warnings {
        switch warning {
        case .pathMissing:
            logger.info("warning type=path_missing")
        case .versionMismatch(let cliVersion, let appVersion):
            logger.info(
                "warning type=version_mismatch cliVersion=\(SetupLogSanitizer.field(cliVersion)) " +
                "appVersion=\(SetupLogSanitizer.field(appVersion))"
            )
        }
    }
}

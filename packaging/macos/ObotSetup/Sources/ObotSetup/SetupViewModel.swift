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
    private let appVersion: String
    private let environmentPath: String?

    init(
        cliClient: ObotCLIClient = ObotCLIClient(),
        setupRunner: any SetupCommandRunning = ProcessSetupRunner(),
        appVersion: String = Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? "dev",
        environmentPath: String? = ProcessInfo.processInfo.environment["PATH"]
    ) {
        self.cliClient = cliClient
        self.setupRunner = setupRunner
        self.appVersion = appVersion
        self.environmentPath = environmentPath
    }

    func load() {
        displayState = .loading
        screen = .status

        let cliClient = cliClient
        let appVersion = appVersion
        let environmentPath = environmentPath

        Task {
            let snapshot = await Task.detached(priority: .userInitiated) {
                loadSetupSnapshot(
                    cliClient: cliClient,
                    appVersion: appVersion,
                    environmentPath: environmentPath
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
    }

    func confirmURL() {
        let trimmed = urlText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard isValidObotURL(trimmed) else {
            urlValidationMessage = "Enter a valid http or https URL."
            return
        }

        urlText = trimmed
        urlValidationMessage = nil
        screen = .agents
    }

    func backToStatus() {
        guard !isRunningSetup else {
            return
        }
        screen = .status
    }

    func backToURL() {
        guard !isRunningSetup else {
            return
        }
        screen = .url
    }

    func toggleAgent(_ agent: DetectedAgent) {
        guard agent.isPresent, !isRunningSetup else {
            return
        }

        if selectedAgentIDs.contains(agent.id) {
            selectedAgentIDs.remove(agent.id)
        } else {
            selectedAgentIDs.insert(agent.id)
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

        Task {
            let result = await setupRunner.runSetup(executableURL: executableURL, request: request) { [weak self] event in
                Task { @MainActor in
                    self?.progress.apply(event)
                }
            }

            if result.succeeded {
                isRunningSetup = false
                screen = .done
                return
            }

            if let error = result.errorDisplay {
                progress.error = error
            }
            isRunningSetup = false
        }
    }

    func cancelSetup() {
        guard isRunningSetup else {
            return
        }
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
    environmentPath: String?
) -> SetupLoadSnapshot {
    let cliExists = cliClient.cliExists()
    guard cliExists else {
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
        let displayState = SetupStateResolver.resolve(cliExists: true, status: status)
        let warnings = SetupWarningResolver.warnings(
            status: status,
            appVersion: appVersion,
            environmentPath: environmentPath
        )

        if case .unsupportedCLI = displayState {
            return SetupLoadSnapshot(
                displayState: displayState,
                agents: [],
                warnings: warnings,
                configuredURL: status.defaultURL
            )
        }

        return SetupLoadSnapshot(
            displayState: displayState,
            agents: (try? cliClient.loadAgents().agents) ?? [],
            warnings: warnings,
            configuredURL: status.defaultURL
        )
    } catch {
        return SetupLoadSnapshot(
            displayState: SetupStateResolver.resolve(
                cliExists: true,
                status: nil,
                statusError: error.localizedDescription
            ),
            agents: [],
            warnings: SetupWarningResolver.warnings(
                status: nil,
                appVersion: appVersion,
                environmentPath: environmentPath
            ),
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

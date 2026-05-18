import Foundation

@MainActor
final class SetupViewModel: ObservableObject {
    @Published private(set) var displayState: SetupDisplayState = .loading
    @Published private(set) var agents: [DetectedAgent] = []
    @Published private(set) var warnings: [SetupWarning] = []

    private let cliClient: ObotCLIClient
    private let appVersion: String
    private let environmentPath: String?

    init(
        cliClient: ObotCLIClient = ObotCLIClient(),
        appVersion: String = Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? "dev",
        environmentPath: String? = ProcessInfo.processInfo.environment["PATH"]
    ) {
        self.cliClient = cliClient
        self.appVersion = appVersion
        self.environmentPath = environmentPath
    }

    func load() {
        displayState = .loading

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
        }
    }
}

private struct SetupLoadSnapshot: Sendable {
    var displayState: SetupDisplayState
    var agents: [DetectedAgent]
    var warnings: [SetupWarning]
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
            )
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
                warnings: warnings
            )
        }

        return SetupLoadSnapshot(
            displayState: displayState,
            agents: (try? cliClient.loadAgents().agents) ?? [],
            warnings: warnings
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
            )
        )
    }
}

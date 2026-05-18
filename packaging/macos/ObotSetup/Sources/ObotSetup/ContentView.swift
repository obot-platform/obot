import SwiftUI

struct ContentView: View {
    @StateObject private var viewModel = SetupViewModel()

    var body: some View {
        VStack(alignment: .leading, spacing: 20) {
            header

            if !viewModel.warnings.isEmpty {
                warningList(viewModel.warnings)
            }

            stateView

            if !viewModel.agents.isEmpty {
                agentList(viewModel.agents)
            }

            Spacer(minLength: 0)
        }
        .padding(28)
        .task {
            viewModel.load()
        }
    }

    private var header: some View {
        HStack(alignment: .center, spacing: 18) {
            LogoView()
                .frame(width: 150, height: 58)

            VStack(alignment: .leading, spacing: 6) {
                Text("Setup")
                    .font(.largeTitle)
                    .fontWeight(.semibold)
                Text("Checking the installed CLI and local setup state.")
                    .foregroundStyle(.secondary)
            }
        }
    }

    @ViewBuilder
    private var stateView: some View {
        switch viewModel.displayState {
        case .loading:
            HStack(spacing: 12) {
                ProgressView()
                Text("Loading setup status")
            }
        case .missingCLI:
            StatusPanel(
                title: "Obot CLI is missing",
                message: "The setup app expected to find \(obotCLIPath). Reinstall Obot or install the CLI before continuing.",
                systemImage: "xmark.octagon"
            )
        case .unsupportedCLI(let version, let missingCapabilities, let message):
            StatusPanel(
                title: "Installed CLI is not supported",
                message: unsupportedCLIMessage(version: version, missingCapabilities: missingCapabilities, message: message),
                systemImage: "exclamationmark.triangle"
            )
        case .firstRun(let status):
            StatusPanel(
                title: "Setup is ready",
                message: "CLI version \(status.version) is installed. No Obot URL is configured yet.",
                systemImage: "checkmark.circle"
            )
        case .alreadyConfigured(let status):
            StatusPanel(
                title: "Obot is configured",
                message: "CLI version \(status.version) is installed and authenticated for \(status.defaultURL).",
                systemImage: "checkmark.circle"
            )
        case .needsLoginRepair(let status):
            StatusPanel(
                title: "Login needs repair",
                message: "The CLI is configured for \(status.defaultURL), but the stored login token is not valid.",
                systemImage: "key"
            )
        }
    }

    private func unsupportedCLIMessage(version: String?, missingCapabilities: [String], message: String?) -> String {
        if let message, !message.isEmpty {
            return message
        }

        let versionText = version.map { "version \($0)" } ?? "the installed version"
        return "Obot \(versionText) is missing required setup capabilities: \(missingCapabilities.joined(separator: ", "))."
    }

    private func warningList(_ warnings: [SetupWarning]) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            ForEach(warnings) { warning in
                HStack(alignment: .top, spacing: 10) {
                    Image(systemName: "exclamationmark.triangle.fill")
                        .foregroundStyle(.yellow)
                    VStack(alignment: .leading, spacing: 2) {
                        Text(warning.title)
                            .font(.subheadline)
                            .fontWeight(.semibold)
                        Text(warning.message)
                            .font(.caption)
                            .foregroundStyle(.secondary)
                            .textSelection(.enabled)
                    }
                }
            }
        }
        .padding(14)
        .background(Color.yellow.opacity(0.12), in: RoundedRectangle(cornerRadius: 8))
    }

    private func agentList(_ agents: [DetectedAgent]) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("Detected local agents")
                .font(.headline)

            VStack(spacing: 0) {
                ForEach(agents) { agent in
                    HStack(spacing: 12) {
                        Image(systemName: agent.isPresent ? "checkmark.circle.fill" : "minus.circle")
                            .foregroundStyle(agent.isPresent ? .green : .secondary)
                        VStack(alignment: .leading, spacing: 2) {
                            Text(agent.displayName)
                                .fontWeight(.medium)
                            Text(agent.reason)
                                .font(.caption)
                                .foregroundStyle(.secondary)
                                .lineLimit(2)
                        }
                        Spacer()
                        Text(agent.state)
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                    .padding(.vertical, 10)

                    if agent.id != agents.last?.id {
                        Divider()
                    }
                }
            }
            .padding(.horizontal, 12)
            .background(Color(nsColor: .controlBackgroundColor), in: RoundedRectangle(cornerRadius: 8))
        }
    }
}

private struct StatusPanel: View {
    var title: String
    var message: String
    var systemImage: String

    var body: some View {
        HStack(alignment: .top, spacing: 14) {
            Image(systemName: systemImage)
                .font(.title2)
                .foregroundStyle(Color.accentColor)
                .frame(width: 28)

            VStack(alignment: .leading, spacing: 6) {
                Text(title)
                    .font(.title3)
                    .fontWeight(.semibold)
                Text(message)
                    .foregroundStyle(.secondary)
                    .textSelection(.enabled)
            }
        }
        .padding(16)
        .background(Color(nsColor: .textBackgroundColor), in: RoundedRectangle(cornerRadius: 8))
        .overlay(
            RoundedRectangle(cornerRadius: 8)
                .stroke(Color(nsColor: .separatorColor), lineWidth: 1)
        )
    }
}

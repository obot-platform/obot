import SwiftUI

struct ContentView: View {
    @StateObject private var viewModel = SetupViewModel()
    @FocusState private var focusedField: FocusedField?

    private enum FocusedField {
        case url
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 20) {
            header

            if !viewModel.warnings.isEmpty {
                warningList(viewModel.warnings)
            }

            screenView

            Spacer(minLength: 0)
        }
        .padding(28)
        .task {
            viewModel.load()
        }
        .onChange(of: viewModel.screen) { screen in
            if screen == .url {
                focusURLField()
            }
        }
    }

    private var header: some View {
        HStack(alignment: .center, spacing: 18) {
            LogoView()
                .frame(width: 50, height: 50)
            Text("Obot Setup")
                .font(.largeTitle)
                .fontWeight(.semibold)
        }
    }

    @ViewBuilder
    private var screenView: some View {
        switch viewModel.screen {
        case .status:
            statusScreen
        case .url:
            urlScreen
        case .agents:
            agentsScreen
        case .running:
            runScreen
        case .done:
            doneScreen
        }
    }

    @ViewBuilder
    private var statusScreen: some View {
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
            VStack(alignment: .leading, spacing: 16) {
                StatusPanel(
                    title: "Setup is ready",
                    message: "CLI version \(status.version) is installed. No Obot URL is configured yet.",
                    systemImage: "checkmark.circle"
                )
                primaryButton("Continue", systemImage: "arrow.right", action: viewModel.startSetupFlow)
            }
        case .alreadyConfigured(let status):
            VStack(alignment: .leading, spacing: 16) {
                StatusPanel(
                    title: "Obot is configured",
                    message: "CLI version \(status.version) is installed and authenticated for \(status.defaultURL).",
                    systemImage: "checkmark.circle"
                )
                primaryButton("Set Up Again", systemImage: "arrow.clockwise", action: viewModel.startSetupFlow)
            }
        case .needsLoginRepair(let status):
            VStack(alignment: .leading, spacing: 16) {
                StatusPanel(
                    title: "Login needs repair",
                    message: "The CLI is configured for \(status.defaultURL), but the stored login token is not valid.",
                    systemImage: "key"
                )
                primaryButton("Repair Login", systemImage: "key", action: viewModel.startSetupFlow)
            }
        }
    }

    private var urlScreen: some View {
        VStack(alignment: .leading, spacing: 16) {
            Text("Obot URL")
                .font(.title2)
                .fontWeight(.semibold)

            TextField("https://obot.example.com", text: $viewModel.urlText)
                .textFieldStyle(.roundedBorder)
                .frame(maxWidth: 460)
                .focused($focusedField, equals: .url)
                .submitLabel(.continue)
                .onSubmit(viewModel.confirmURL)
                .onAppear(perform: focusURLField)

            if let message = viewModel.urlValidationMessage {
                Label(message, systemImage: "exclamationmark.circle")
                    .foregroundStyle(.red)
                    .font(.caption)
            }

            HStack(spacing: 12) {
                Button("Back", action: viewModel.backToStatus)
                primaryButton("Continue", systemImage: "arrow.right", action: viewModel.confirmURL)
                    .keyboardShortcut(.defaultAction)
            }
        }
    }

    private var agentsScreen: some View {
        VStack(alignment: .leading, spacing: 16) {
            Text("Local Agents")
                .font(.title2)
                .fontWeight(.semibold)

            agentSelectionList(viewModel.agents)

            HStack(spacing: 12) {
                Button("Back", action: viewModel.backToURL)
                primaryButton("Run Setup", systemImage: "play.fill", action: viewModel.runSetup)
            }
        }
    }

    private var runScreen: some View {
        VStack(alignment: .leading, spacing: 16) {
            HStack(spacing: 12) {
                if viewModel.isRunningSetup {
                    ProgressView()
                        .controlSize(.small)
                }
                Text(viewModel.isRunningSetup ? "Running setup" : "Setup stopped")
                    .font(.title2)
                    .fontWeight(.semibold)
            }

            progressList

            if let error = viewModel.progress.error {
                StatusPanel(title: error.title, message: error.message, systemImage: "exclamationmark.triangle")
            }

            HStack(spacing: 12) {
                if viewModel.isRunningSetup {
                    Button("Cancel", role: .destructive, action: viewModel.cancelSetup)
                } else {
                    Button("Back", action: viewModel.backToURL)
                    primaryButton("Run Again", systemImage: "arrow.clockwise", action: viewModel.runSetup)
                }
            }
        }
    }

    private var doneScreen: some View {
        VStack(alignment: .leading, spacing: 16) {
            StatusPanel(
                title: "Setup complete",
                message: "Obot is installed at \(obotCLIPath) and configured for \(viewModel.urlText).",
                systemImage: "checkmark.circle"
            )
            HStack(spacing: 12) {
                primaryButton("Run Setup Again", systemImage: "arrow.clockwise", action: viewModel.startSetupFlow)
            }
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

    private func agentSelectionList(_ agents: [DetectedAgent]) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            VStack(spacing: 0) {
                if agents.isEmpty {
                    Text("No supported local agents were detected.")
                        .foregroundStyle(.secondary)
                        .padding(.vertical, 12)
                } else {
                    ForEach(agents) { agent in
                        Button {
                            viewModel.toggleAgent(agent)
                        } label: {
                            HStack(spacing: 12) {
                                Image(systemName: selectionImageName(for: agent))
                                    .foregroundStyle(agent.isPresent ? Color.accentColor : .secondary)
                                    .frame(width: 20)

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
                            .contentShape(Rectangle())
                            .padding(.vertical, 10)
                        }
                        .buttonStyle(.plain)
                        .disabled(!agent.isPresent)

                        if agent.id != agents.last?.id {
                            Divider()
                        }
                    }
                }
            }
            .padding(.horizontal, 12)
            .background(Color(nsColor: .controlBackgroundColor), in: RoundedRectangle(cornerRadius: 8))
        }
    }

    private var progressList: some View {
        VStack(alignment: .leading, spacing: 10) {
            if viewModel.progress.events.isEmpty {
                Text("Waiting for Obot to start.")
                    .foregroundStyle(.secondary)
                    .padding(.vertical, 12)
            } else {
                ForEach(viewModel.progress.events) { event in
                    HStack(alignment: .top, spacing: 12) {
                        Image(systemName: progressImageName(for: event))
                            .foregroundStyle(progressColor(for: event))
                            .frame(width: 20)
                        VStack(alignment: .leading, spacing: 2) {
                            Text(progressTitle(for: event))
                                .fontWeight(.medium)

                            if let detail = progressDetail(for: event) {
                                Text(detail)
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                                    .lineLimit(3)
                                    .textSelection(.enabled)
                            }
                        }
                        Spacer()
                    }
                    .padding(.vertical, 8)
                }
            }
        }
        .padding(.horizontal, 12)
        .background(Color(nsColor: .controlBackgroundColor), in: RoundedRectangle(cornerRadius: 8))
    }

    private func primaryButton(_ title: String, systemImage: String, action: @escaping () -> Void) -> some View {
        Button(action: action) {
            Label(title, systemImage: systemImage)
        }
        .buttonStyle(.borderedProminent)
    }

    private func focusURLField() {
        DispatchQueue.main.async {
            focusedField = .url
        }
    }

    private func selectionImageName(for agent: DetectedAgent) -> String {
        guard agent.isPresent else {
            return "minus.circle"
        }
        return viewModel.selectedAgentIDs.contains(agent.id) ? "checkmark.square.fill" : "square"
    }

    private func progressImageName(for event: SetupProgressEvent) -> String {
        switch event.type {
        case "complete":
            return "checkmark.circle.fill"
        case "error":
            return "exclamationmark.triangle.fill"
        default:
            return "circle.fill"
        }
    }

    private func progressColor(for event: SetupProgressEvent) -> Color {
        switch event.type {
        case "complete":
            return .green
        case "error":
            return .red
        default:
            return .accentColor
        }
    }

    private func progressTitle(for event: SetupProgressEvent) -> String {
        switch event.type {
        case "auth_started":
            return "Opening browser login"
        case "auth_completed":
            return "Login complete"
        case "config_saved":
            return "Configuration saved"
        case "agent_installed":
            return "\(event.displayName ?? "Local agent") configured"
        case "complete":
            return "Setup complete"
        case "error":
            return SetupErrorDisplay.from(event: event).title
        default:
            return event.type
        }
    }

    private func progressDetail(for event: SetupProgressEvent) -> String? {
        switch event.type {
        case "auth_started", "auth_completed", "config_saved", "complete":
            return event.url
        case "agent_installed":
            return event.message
        case "error":
            return event.message
        default:
            return event.message
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

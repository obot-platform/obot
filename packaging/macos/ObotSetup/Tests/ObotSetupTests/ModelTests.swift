import XCTest
@testable import ObotSetup

final class ModelTests: XCTestCase {
    func testDecodesSetupStatusJSON() throws {
        let json = """
        {
          "version": "v0.21.0",
          "capabilities": [
            "setup.nonInteractive",
            "setup.detectAgents",
            "setup.progressJSON"
          ],
          "defaultURL": "https://obot.example.com",
          "tokenValid": true,
          "setupComplete": true
        }
        """

        let status = try JSONDecoder().decode(SetupStatus.self, from: Data(json.utf8))

        XCTAssertEqual(status.version, "v0.21.0")
        XCTAssertEqual(status.defaultURL, "https://obot.example.com")
        XCTAssertTrue(status.tokenValid)
        XCTAssertTrue(status.setupComplete)
        XCTAssertTrue(status.capabilitySet.contains("setup.progressJSON"))
    }

    func testDecodesAgentDetectionJSON() throws {
        let json = """
        {
          "agents": [
            {
              "id": "claude-code",
              "displayName": "Claude Code",
              "state": "present",
              "reason": "Found Claude Code config directory"
            },
            {
              "id": "cursor",
              "displayName": "Cursor",
              "state": "missing",
              "reason": "Cursor config directory was not found"
            }
          ]
        }
        """

        let output = try JSONDecoder().decode(AgentDetectionOutput.self, from: Data(json.utf8))

        XCTAssertEqual(output.agents.count, 2)
        XCTAssertEqual(output.agents[0].id, "claude-code")
        XCTAssertTrue(output.agents[0].isPresent)
        XCTAssertEqual(output.agents[1].displayName, "Cursor")
        XCTAssertFalse(output.agents[1].isPresent)
    }

    func testSetupRunRequestBuildsNonInteractiveJSONCommand() {
        let request = SetupRunRequest(
            url: "https://obot.example.com",
            selectedAgentIDs: ["claude-code", "cursor"]
        )

        XCTAssertEqual(
            request.arguments,
            [
                "setup",
                "--url", "https://obot.example.com",
                "--agents", "claude-code,cursor",
                "--yes",
                "--non-interactive",
                "--output", "json",
            ]
        )
    }

    func testSetupRunRequestUsesAgentsNoneWhenSelectionIsEmpty() {
        let request = SetupRunRequest(url: "https://obot.example.com", selectedAgentIDs: [])

        XCTAssertEqual(request.agentArgument, "none")
        XCTAssertEqual(request.arguments[4], "none")
    }

    func testProgressStateHandlesSuccessfulEvents() {
        var progress = SetupProgressState()

        progress.apply(SetupProgressEvent(
            type: "auth_started",
            code: nil,
            message: nil,
            url: "https://obot.example.com",
            agentID: nil,
            displayName: nil,
            installed: nil
        ))
        progress.apply(SetupProgressEvent(
            type: "complete",
            code: nil,
            message: nil,
            url: "https://obot.example.com",
            agentID: nil,
            displayName: nil,
            installed: nil
        ))

        XCTAssertEqual(progress.events.map(\.type), ["auth_started", "complete"])
        XCTAssertTrue(progress.isComplete)
        XCTAssertNil(progress.error)
    }

    func testProgressStateMapsStructuredErrorDisplay() {
        var progress = SetupProgressState()

        progress.apply(SetupProgressEvent(
            type: "error",
            code: "server_unreachable",
            message: "dial tcp: no such host",
            url: nil,
            agentID: nil,
            displayName: nil,
            installed: nil
        ))

        XCTAssertEqual(progress.error?.code, "server_unreachable")
        XCTAssertEqual(progress.error?.title, "Obot server is unreachable")
        XCTAssertEqual(progress.error?.message, "dial tcp: no such host")
    }

    func testStateResolverReportsMissingCLI() {
        let state = SetupStateResolver.resolve(cliExists: false, status: nil)

        XCTAssertEqual(state, .missingCLI)
    }

    func testStateResolverReportsUnsupportedCLIWhenStatusCommandFails() {
        let state = SetupStateResolver.resolve(
            cliExists: true,
            status: nil,
            statusError: "unknown command \"status\""
        )

        XCTAssertEqual(
            state,
            .unsupportedCLI(
                version: nil,
                missingCapabilities: SetupStateResolver.requiredCapabilities,
                message: "unknown command \"status\""
            )
        )
    }

    func testStateResolverReportsUnsupportedCLIWhenCapabilitiesAreMissing() {
        let status = SetupStatus(
            version: "v0.20.0",
            capabilities: ["setup.nonInteractive"],
            defaultURL: "",
            tokenValid: false,
            setupComplete: false
        )

        let state = SetupStateResolver.resolve(cliExists: true, status: status)

        XCTAssertEqual(
            state,
            .unsupportedCLI(
                version: "v0.20.0",
                missingCapabilities: ["setup.detectAgents", "setup.progressJSON"],
                message: nil
            )
        )
    }

    func testStateResolverReportsFirstRun() {
        let status = supportedStatus(defaultURL: "", tokenValid: false, setupComplete: false)

        let state = SetupStateResolver.resolve(cliExists: true, status: status)

        XCTAssertEqual(state, .firstRun(status: status))
    }

    func testStateResolverReportsAlreadyConfigured() {
        let status = supportedStatus(
            defaultURL: "https://obot.example.com",
            tokenValid: true,
            setupComplete: true
        )

        let state = SetupStateResolver.resolve(cliExists: true, status: status)

        XCTAssertEqual(state, .alreadyConfigured(status: status))
    }

    func testStateResolverReportsLoginRepair() {
        let status = supportedStatus(
            defaultURL: "https://obot.example.com",
            tokenValid: false,
            setupComplete: false
        )

        let state = SetupStateResolver.resolve(cliExists: true, status: status)

        XCTAssertEqual(state, .needsLoginRepair(status: status))
    }

    func testWarningResolverReportsPathAndVersionWarnings() {
        let status = supportedStatus(
            version: "v0.21.0",
            defaultURL: "https://obot.example.com",
            tokenValid: true,
            setupComplete: true
        )

        let warnings = SetupWarningResolver.warnings(
            status: status,
            appVersion: "v0.22.0",
            environmentPath: "/opt/homebrew/bin:/usr/bin"
        )

        XCTAssertEqual(warnings.count, 2)
        XCTAssertEqual(warnings[0], .pathMissing(path: "/opt/homebrew/bin:/usr/bin"))
        XCTAssertEqual(warnings[1], .versionMismatch(cliVersion: "v0.21.0", appVersion: "v0.22.0"))
    }

    func testWarningResolverSkipsWarningsWhenPathAndVersionMatch() {
        let status = supportedStatus(
            version: "v0.21.0",
            defaultURL: "https://obot.example.com",
            tokenValid: true,
            setupComplete: true
        )

        let warnings = SetupWarningResolver.warnings(
            status: status,
            appVersion: "v0.21.0",
            environmentPath: "/usr/local/bin:/usr/bin"
        )

        XCTAssertTrue(warnings.isEmpty)
    }

    func testLocalSetupLoggerOverwritesLogOnReset() throws {
        let logURL = FileManager.default.temporaryDirectory
            .appendingPathComponent(UUID().uuidString, isDirectory: true)
            .appendingPathComponent("setup.log", isDirectory: false)
        let logger = LocalSetupLogger(fileURL: logURL)

        logger.resetRun(appVersion: "v1")
        logger.info("first_run_marker")
        logger.resetRun(appVersion: "v2")
        logger.info("second_run_marker")

        let log = try String(contentsOf: logURL, encoding: .utf8)
        XCTAssertFalse(log.contains("first_run_marker"))
        XCTAssertTrue(log.contains("app_started appVersion=v2"))
        XCTAssertTrue(log.contains("second_run_marker"))
    }

    func testSetupLogSanitizerRedactsSecretsAndSensitivePaths() {
        let sanitized = SetupLogSanitizer.message(
            "failed https://user:pass@obot.example.com/setup?token=abc " +
            "Bearer abc.def token=secret " +
            "/Users/alice/.config/obot/config.json " +
            "/private/var/folders/aa/bb/T/output"
        )

        XCTAssertTrue(sanitized.contains("https://obot.example.com"))
        XCTAssertFalse(sanitized.contains("user:pass"))
        XCTAssertFalse(sanitized.contains("?token=abc"))
        XCTAssertFalse(sanitized.contains("abc.def"))
        XCTAssertFalse(sanitized.contains("token=secret"))
        XCTAssertFalse(sanitized.contains("alice"))
        XCTAssertFalse(sanitized.contains(".config/obot/config.json"))
        XCTAssertFalse(sanitized.contains("/private/var/folders/aa"))
    }

    private func supportedStatus(
        version: String = "v0.21.0",
        defaultURL: String,
        tokenValid: Bool,
        setupComplete: Bool
    ) -> SetupStatus {
        SetupStatus(
            version: version,
            capabilities: SetupStateResolver.requiredCapabilities,
            defaultURL: defaultURL,
            tokenValid: tokenValid,
            setupComplete: setupComplete
        )
    }
}

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

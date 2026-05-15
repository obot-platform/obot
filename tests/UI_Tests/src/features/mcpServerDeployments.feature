Feature: Manage <ServerName> MCP Server deployments and connections on Obot

  Background: Navigate to Obot
    Given I setup context for assertion
    When User navigates to the Obot main login page

  @regression
  Scenario Outline: Connect, restart and delete MCP server "<ServerName>" on Obot
    When User searches for MCP server "<ConnectionName>"
    And User performs "Connect To Server" action on MCP server "<ConnectionName>"
    And User connects to the "<ConnectionName>" MCP server
    Then User closes the MCP server connection modal

    When User navigate to deployments and connections tab
    And User performs "Restart Server" action on MCP server "<ConnectionName>"
    And User selects "<ConnectionName>" MCP server
    Then "<ConnectionName>" MCP server should be restarted successfully

    When User navigates back to MCP server list
    And User performs "Delete Server" action on MCP server "<ConnectionName>"
    And User confirms deletion of MCP server "<ConnectionName>"
    Then MCP server "<ConnectionName>" should be deleted successfully

    Examples:
      | ServerName        | ConnectionName | PromptName   | ReportName   | ConnectionNameUpdated |
      | test-antv_charts  | AntV Charts    | AntV Charts  | AntV Charts  | AntV Charts Updated   |
      | test-aws          | AWS API        | AWS API      | AWS API      | AWS API Updated       |
      | test-brave_search | Brave Search   | Brave Search | Brave Search | Brave Search Updated  |
      | test-git lab      | GitLab         | GitLab       | GitLab       | GitLab Updated        |
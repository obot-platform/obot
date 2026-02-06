Feature: Update Composite <ConnectionName> MCP Server on Obot

  Background: Navigate to Obot
    Given I setup context for assertion
    When User navigates to the Obot main login page

  @regression
  Scenario Outline: Update composite MCP server "<ConnectionName>" on Obot
    When User clicks on "Add MCP Server" button
    And User selects "Composite Server" option from select server type modal
    And User enters "<ConnectionName>" in MCP server name field
    And User add "<ComponentServers>" component servers to composite server
    And User save the composite MCP server
    And User skip connect to "<ConnectionName>" MCP server

    And User navigates back to MCP server list
    And User searches for MCP server "<ConnectionName>"
    Then MCP server "<ConnectionName>" should be added successfully

    When User selects "<ConnectionName>" MCP server
    And User selects "Connect To Server" button
    And User Connect to "<ConnectionName>" MCP server
    Then User closes the MCP server connection modal

    When User open "Configuration" tab
    And User delete existing component server "<DeleteServer>" from composite MCP server
    And User navigates back to MCP server list
    When User navigate to deployments and connections tab
    Then "<ComponentServers>" component servers of "<ConnectionName>" composite MCP server should be displayed

    When User performs "View Diff" action on MCP server "<ConnectionName>"
    Then Configuration diff modal should be displayed for "<ConnectionName>" composite MCP server

    And User searches for MCP server "<ConnectionName>"
    When User performs "Update Server" action on MCP server "<ConnectionName>"
    And User confirms update of composite MCP server
    Then "<DeleteServer>" component server should be deleted from "<ConnectionName>" composite MCP server
    And "View Diff,Update Server" action on MCP server "<ConnectionName>" should not be available

    When User goes back to MCP server list
    And User performs "Disconnect" action on MCP server "<ConnectionName>"
    Then MCP server "<ConnectionName>" should be disconnected successfully

    When User searches for MCP server "<ConnectionName>"
    And User performs "Delete Entry" action on MCP server "<ConnectionName>"
    And User confirms deletion of MCP server "<ConnectionName>"
    Then MCP server "<ConnectionName>" should be deleted successfully

    Examples:
      | ConnectionName            | ComponentServers                                                                | DeleteServer |
      | AntV Exa Composite Server | AntV Charts,Exa Search                                                          | Exa Search   |
      | AWS Composite Server      | AWS API,AWS CDK,AWS Documentation,AWS EKS,AWS Kendra,AWS Knowledge,AWS Redshift | AWS Redshift |
Feature: Manage <ServerName> MCP Server configuration on Obot

    Background: Navigate to Obot
        Given I setup context for assertion
        When User navigates to the Obot main login page

    @regression
    Scenario Outline: Update config, rename, verify details and disconnect MCP server "<ServerName>" on Obot
        When User searches for MCP server "<ConnectionName>"
        And User performs "Connect To Server" action on MCP server "<ConnectionName>"
        And User connects to the "<ConnectionName>" MCP server
        Then User closes the MCP server connection modal
        
        When User performs "Edit Configuration" action on MCP server "<ConnectionName>"
        And User updates MCP server config for "<ConnectionName>" and saves
        And User performs "Edit Configuration" action on MCP server "<ConnectionName>"
        Then MCP server config should be updated for "<ConnectionName>"

        When User performs "Rename" action on MCP server "<ConnectionName>"
        And User updates MCP server name to "<ConnectionNameUpdated>"

        And User performs "Server Details" action on MCP server "<ConnectionName>"
        Then MCP server name should be updated to "<ConnectionNameUpdated>" on details page
        And MCP server details for "<ConnectionName>" should be correct

        When User navigates back to MCP server list
        
        And User performs "Chat" action on MCP server "<ConnectionName>"
        And User sends prompts to Obot AI chat for "<PromptName>" MCP server
        Then All prompt results should be validated and a report generated for the selected "<ReportName>" MCP server

        When User goes back to MCP server list

        And User performs "Disconnect" action on MCP server "<ConnectionName>"
        Then MCP server "<ConnectionName>" should be disconnected successfully

        Examples:
            | ServerName                       | ConnectionName                     | PromptName       | ReportName       | ConnectionNameUpdated        |
            | test-antv_charts                 | AntV Charts                        | AntV Charts      | AntV Charts      | AntV Charts Updated          |
            | test-aws                         | AWS API                            | AWS API          | AWS API          | AWS API Updated            |
            | test-brave_search                | Brave Search                       | Brave Search     | Brave Search     | Brave Search Updated       |
            | test-git lab                     | GitLab                             | GitLab           | GitLab           | GitLab Updated            |
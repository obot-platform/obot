Feature: Add Composite <ServerName> MCP Server on Obot

    Background: Navigate to Obot
        Given I setup context for assertion
        When User navigates to the Obot main login page

    @regression
    Scenario Outline: Add composite MCP server "<ConnectionName>" on Obot
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
        When User navigates back to MCP server list

        And User performs "Chat" action on MCP server "<ConnectionName>"
        And User sends prompts to Obot AI chat for "<PromptName>" MCP server
        Then All prompt results should be validated and a report generated for the selected "<ReportName>" MCP server

        When User goes back to MCP server list
        And User performs "Disconnect" action on MCP server "<ConnectionName>"
        Then MCP server "<ConnectionName>" should be disconnected successfully

        When User searches for MCP server "<ConnectionName>"
        And User performs "Delete Entry" action on MCP server "<ConnectionName>"
        And User confirms deletion of MCP server "<ConnectionName>"
        Then MCP server "<ConnectionName>" should be deleted successfully

        Examples:
            | ConnectionName                    | ComponentServers                                                                | PromptName                 | ReportName                 |
            | AntV Exa Composite Server         | AntV Charts,Exa Search                                                          | AntV Exa Composite         | AntV Exa Composite         |
            # | AWS Composite Server              | AWS API,AWS CDK,AWS Documentation,AWS EKS,AWS Kendra,AWS Knowledge,AWS Redshift | AWS Composite              | AWS Composite              |
            # | Gitlab Wordpress Composite Server | GitLab,WordPress                                                                | GitLab WordPress Composite | GitLab WordPress Composite |

Feature: Connecte MCP server on Obot

    Background: Navigate to Obot
        Given I setup context for assertion
        When User navigates the Obot main login page
        Then User open chat Obot

    Scenario: Validate Wordpress sequential prompts on Obot
        When User open MCP connector page
        And User select "WordPress1" MCP server
        And User select "Connect To Server" button
        And User connect to the WordPress1 MCP server 
        When User sends following prompts to Obot AI chat for "Wordpress" MCP server:
          | prompt                                                                                                                                                                                      |
          | Is Wordpress tool connected?                                                                                                                                                                |
          | Validate the connection credentials for the WordPress server.                                                                                                                               |
          | Retrieve the current WordPress site settings.                                                                                                                                               |
          | Get details of the connected WordPress user.                                                                                                                                                |
          | Create a new WordPress post with the title 'MCP QA Testing Guide' and content 'Testing all endpoints for the WordPress MCP connector.'Set status to 'draft' and assign category 'QA Tests.' |
          | List the 5 most recent published posts.                                                                                                                                                     |
        Then All prompts results should be validated and report generated for selected "Wordpress" MCP Server

    Scenario: Validate GitLab sequential prompts on Obot 
        When User open MCP connector page
        And User select "GitLab" MCP server
        And User select "Connect To Server" button
        And User connect to the GitLab MCP server
        When User sends following prompts to Obot AI chat for "Gitlab" MCP server:
          | prompt                                                                                                                                               |
          | Is GitLab tool connected?                                                                                                                            |
          | List all projects available in my GitLab account.                                                                                                    |
          | Create a new project named GitLab_Test_AUtomation with description Testing Automation MCP API integration.                                           |
          | Update the project GitLab_Test_AUtomation to make it private.                                                                                        |
          | Create a new branch named feature/test-mcp-automation from the main branch in the GitLab_Test_AUtomation project.                                    |
          | List all branches in the project GitLab_Test_AUtomation.                                                                                             |
          | Get branch diffs between feature/test-mcp-automation and main.                                                                                       |
          | Create a new file named test.txt in the main branch with content: Hello MCP GitLab!.                                                                 |
          | Update the file test.txt in the main branch with content: File updated via MCP server.                                                               |
          Then All prompts results should be validated and report generated for selected "Gitlab" MCP Server
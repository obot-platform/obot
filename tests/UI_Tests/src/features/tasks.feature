Feature: Create and Run On Demand tasks on Obot

  Background: Navigate to Obot
    Given I setup context for assertion
    When User navigates to the Obot main login page
    Then User opens chat Obot
    And User creates a new Project with no existing connections

  @regression
  Scenario: Create and run On Demand task on Obot
    When User clicks on "Start New Task" button
    And User enters "Sum of two numbers and square" in Task name field
    And User enters description in Task description field
    And User selects "On Demand" option from Task type dropdown
    And User adds an arguments to the On Demand task
    And User adds steps to the On Demand task
    And User clicks on "Run" button to run the On Demand task
    And User inputs arguments for the On Demand task
    And User clicks on "Run" button in Run Task dialog
    Then Vaidate Task results for the On Demand task

    When User clicks on "Toggle Chat" button to open Obot chat panel
    Then Obot chat panel should be opened successfully

    When User sends prompt "summarize the task results" to Obot in Task chat panel
    Then Validate that Obot responds with a result

    When User clicks on "Delete Task" button to delete the On Demand task
    Then On Demand task should be deleted successfully

# WebdriverIO Automation Project
This project uses **WebdriverIO** and **JavaScript** to automate the testing of **Obot.ai**, an AI-driven chatbot platform. The automation suite includes tests for logging into the platform, creating and editing AI agents, managing tasks, and using tools like file uploads, task management, and message handling.

## Table of Contents
- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
- [Running the Tests](#running-the-tests)
- [Contributing](#contributing)

## Prerequisites
Before running the tests, make sure you have the following installed:

- [Node.js](https://nodejs.org/en/) (Version 14.x or higher)
- [npm](https://www.npmjs.com/) (Node package manager)
- Web browser (e.g., Chrome, etc.)
- **ChromeDriver** for WebDriver support in respective browsers.

## Quick Start

1. Run `npm install` to install the necessary dependencies.
2. Run all tests with `npm run wdio:all`.

## Running the tests

Once you've installed the dependencies and set up your environment, you can run the tests as follows:

1. **Run all tests**:
   To run the entire automation test suite, execute:
   ```bash
   npm run wdio:all
   ```

2. **Run a specific test**:
   If you want to run a specific scenario or feature, use:
   ```bash
   npx wdio run wdio.conf.ts --cucumberOpts.tagExpression='@gitlab'
   ```
   Replace `@gitlab` with the tag corresponding to the scenario you want to execute.

3. **Run in headless mode** (useful for CI/CD):
   You can run the tests in headless mode (without opening the browser window) by configuring the `wdio.conf.ts` file to enable headless execution, and then run the tests with:
   ```bash
   npm run wdio:all
   ```

4. **View results**:
   After the tests are executed, you can review the results in the console or check the generated Allure reports if they are configured.

## Usage
This test suite is designed to work with WebDriverIO and Cucumber for behavior-driven testing. The tests validate user interactions with the Obot.ai platform, such as logging in, creating AI agents, managing tasks, and using different tools integrated into the platform.

## Contributing

If you would like to contribute to this project, please follow these steps:
1. Fork the repository.
2. Create a new branch (`git checkout -b feature-branch`).
3. Make your changes and commit them (`git commit -m 'Add new feature'`).
4. Push to the branch (`git push origin feature-branch`).
5. Create a new Pull Request.

//
// =====
// Hooks
// =====
// WebdriverIO provides a several hooks you can use to interfere the test process in order to
// enhance it and build services around it. You can either apply a single function to it or
// an array of methods. If one of them returns with a promise,
// WebdriverIO will wait until that promise is resolved to continue.
//
export const hooks = {
    /**
     * Gets executed once before all workers get launched.
     * @param {Object} config wdio configuration object
     * @param {Array.<Object>} capabilities list of capabilities details
     */
    // onPrepare: function (config, capabilities) {
    // },
    /**
     * Gets executed before a worker process is spawned & can be used to initialize specific service
     * for that worker as well as modify runtime environments in an async fashion.
     * @param  {String} cid    capability id (e.g 0-0)
     * @param  {[type]} caps   object containing capabilities for session
     * @param  {[type]} specs  specs to be run in the worker process
     * @param  {[type]} args   object that will be merged with the main
     *                         configuration once worker is initialized
     * @param  {[type]} execArgv list of string arguments passed to the worker process
     */
    // onWorkerStart: function (cid, caps, specs, args, execArgv) {
    // },
    /**
     * Gets executed just before initializing the webdriver session and test framework.
     * It allows you to manipulate configurations depending on the capability or spec.
     * @param {Object} config wdio configuration object
     * @param {Array.<Object>} capabilities list of capabilities details
     * @param {Array.<String>} specs List of spec file paths that are to be run
     */
    // beforeSession: function (config, capabilities, specs) {
    // },
    /**
     * Gets executed before test execution begins. At this point you can access to all global
     * variables like `browser`. It is the perfect place to define custom commands.
     * @param {Array.<Object>} capabilities list of capabilities details
     * @param {Array.<String>} specs List of spec file paths that are to be run
     */
    // before: function (capabilities, specs) {
    // },
    /**
     * Gets executed before the suite starts.
     * @param {Object} suite suite details
     */
    // beforeSuite: function (suite) {
    // },
    /**
     * This hook gets executed _before_ every hook within the suite starts.
     * (For example, this runs before calling `before`, `beforeEach`, `after`)
     *
     * (`stepData` and `world` are Cucumber-specific.)
     *
     */
    // beforeHook: function (test, context, stepData, world) {
    // },
    /**
     * Hook that gets executed _after_ every hook within the suite ends.
     * (For example, this runs after calling `before`, `beforeEach`, `after`, `afterEach` in Mocha.)
     *
     * (`stepData` and `world` are Cucumber-specific.)
     */
    // afterHook:function(test,context,{error, result, duration, passed, retries}, stepData,world) {
    // },
    /**
     * Function to be executed before a test (in Mocha/Jasmine) starts.
     */
    // beforeTest: function (test, context) {
    // },
    /**
     * Runs before a WebdriverIO command is executed.
     * @param {String} commandName hook command name
     * @param {Array} args arguments that the command would receive
     */
    // beforeCommand: function (commandName, args) {
    // },
    /**
     * Runs after a WebdriverIO command gets executed
     * @param {String} commandName hook command name
     * @param {Array} args arguments that command would receive
     * @param {Number} result 0 - command success, 1 - command error
     * @param {Object} error error object, if any
     */
    // afterCommand: function (commandName, args, result, error) {
    // },
    /**
     * Function to be executed after a test (in Mocha/Jasmine)
     */
    // afterTest: function (test, context, {error, result, duration, passed, retries}) {
    // },
    /**
     * Hook that gets executed after the suite has ended.
     * @param {Object} suite suite details
     */
    // afterSuite: function (suite) {
    // },
    /**
     * Gets executed after all tests are done. You still have access to all global variables from
     * the test.
     * @param {Number} result 0 - test pass, 1 - test fail
     * @param {Array.<Object>} capabilities list of capabilities details
     * @param {Array.<String>} specs List of spec file paths that ran
     */
    // after: function (result, capabilities, specs) {
    // },
    /**
     * Gets executed right after terminating the webdriver session.
     * @param {Object} config wdio configuration object
     * @param {Array.<Object>} capabilities list of capabilities details
     * @param {Array.<String>} specs List of spec file paths that ran
     */
    // afterSession: function (config, capabilities, specs) {
    // },
    /**
     * Gets executed after all workers have shut down and the process is about to exit.
     * An error thrown in the `onComplete` hook will result in the test run failing.
     * @param {Object} exitCode 0 - success, 1 - fail
     * @param {Object} config wdio configuration object
     * @param {Array.<Object>} capabilities list of capabilities details
     * @param {<Object>} results object containing test results
     */
    // onComplete: function (exitCode, config, capabilities, results) {
    // },
    /**
     * Gets executed when a refresh happens.
     * @param {String} oldSessionId session ID of the old session
     * @param {String} newSessionId session ID of the new session
     */
    // onReload: function (oldSessionId, newSessionId) {
    // },
    /**
     * Cucumber-specific hooks
     */
    // beforeFeature: function (uri, feature, scenarios) {
    // },
    // beforeScenario: function (uri, feature, scenario, sourceLocation) {
    // },
    // beforeStep: function ({uri, feature, step}, context) {
    // },
    afterStep: async function (step, scenario, result, context) {
      if (!result.passed) {
        await browser.takeScreenshot()
      }
    },
    // afterScenario: function (uri, feature, scenario, result, sourceLocation) {
    // },
    // afterFeature: function (uri, feature, scenarios) {
    // }
    /**
   *
   * Runs after a Cucumber Scenario.
   * @param {ITestCaseHookParameter} world            world object containing information on pickle and test step
   * @param {Object}                 result           results object containing scenario results
   * @param {boolean}                result.passed    true if scenario has passed
   * @param {string}                 result.error     error stack if scenario failed
   * @param {number}                 result.duration  duration of scenario in milliseconds
   * @param {Object}                 context          Cucumber World object
   */
  async afterScenario() {
    await browser.reloadSession();
  },
};

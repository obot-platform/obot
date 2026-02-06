import { When, Then, Given } from "@wdio/cucumber-framework";
import Selectors from "../core/selectors";
import {
  clickToElement,
  isElementDisplayed,
  slowInputFilling,
} from "../core/func";
import { LONG_PAUSE, MEDIUM_PAUSE, SHORT_PAUSE } from "../core/timeouts";
import {
  aggregateToolResponses,
  saveMCPReport,
  sendPromptValidateAndCollect,
} from "../core/mcpFunc";
import path from "path";
import { promises as fs } from "fs";
import { generateProjectName } from "../utils/projectName.model";

Given(/^User navigates to the Obot main login page$/, async () => {
  const url = process.env.OBOT_URL;
  await browser.url(url);
});

Then(/^User opens chat Obot$/, async () => {
  await browser.pause(MEDIUM_PAUSE);
  if(await $(Selectors.MCP.adminNavigation.welcomeToObotTitle).isDisplayed()) {
    await clickToElement(Selectors.MCP.popupContinueButton);
  }
  await clickToElement(Selectors.MCP.navigationbtn);
  await clickToElement(Selectors.MCP.clickChatObot);
});

Then(/^User selects "([^"]*)" MCP server$/, async (MCPServer) => {
  await slowInputFilling(Selectors.MCP.mcpSearchInput, MCPServer);
  await isElementDisplayed(
    Selectors.MCP.selectMCPServer(MCPServer),
    LONG_PAUSE,
  );
  // Wait until matching elements appear
  const allServers = await $$(Selectors.MCP.selectMCPServer(MCPServer));
  if ((await allServers.length) === 0)
    throw new Error(`No MCP server found matching: ${MCPServer}`);

  // Click the last one
  const lastServer = allServers[(await allServers.length) - 1];
  await lastServer.waitForDisplayed({ timeout: LONG_PAUSE });
  await lastServer.click();

  await browser.pause(SHORT_PAUSE);
});

Then(/^User selects "([^"]*)" button$/, async (Button) => {
  await isElementDisplayed(Selectors.MCP.btnClick(Button), SHORT_PAUSE);
  await clickToElement(Selectors.MCP.btnClick(Button));
  await browser.pause(SHORT_PAUSE);
  if(await $(Selectors.MCP.btnClick("Connect New Server")).isDisplayed()) {
    await clickToElement(Selectors.MCP.btnClick("Connect New Server"));
  }
});

When(
  /^User sends prompts to Obot AI chat for "([^"]*)" MCP server$/,
  { timeout: 15 * 60 * 1000 },
  async function (serverName: string) {
    const jsonPath = path.resolve(
      process.cwd(),
      "src",
      "data",
      `${serverName.toLowerCase().replace(/\s+/g, "_")}.MCP.json`,
    );
    const data = await fs.readFile(jsonPath, "utf-8");
    const { prompts, tools } = JSON.parse(data);

    this.promptResults = [];
    const toolList = tools;

    for (let i = 0; i < prompts.length; i++) {
      try {
        const result = await sendPromptValidateAndCollect(
          prompts[i],
          toolList,
          i,
        );
        this.promptResults.push(result);
      } catch (err: any) {
        console.error(`Error in prompt #${i + 1}: ${err.message}`);
        this.promptResults.push({ prompt: prompts[i], error: err.message });
      }
    }
    
    // recovery pause after all prompts to let browser settle
    await browser.pause(MEDIUM_PAUSE);
  },
);

Then(
  /^All prompt results should be validated and a report generated for the selected "(.*)" MCP server$/,
  async function (serverName: string) {
    const report = aggregateToolResponses(this.promptResults);
    saveMCPReport(serverName, report);

    const errors = this.promptResults.filter((r) => r.error);
    if (errors.length > 0) {
      console.warn(`${errors.length} prompts had issues.`);
    }
  },
);

Then(/^User connects to the "(.*)" MCP server$/, async (mcpServer: string) => {
  let switchLaunch = false;

  switch (mcpServer.toLowerCase()) {
    case "wordpress":
      await slowInputFilling('//input[contains(@id, "WORDPRESS_SITE")]', process.env.WP_URL);
      await slowInputFilling('//input[contains(@id, "WORDPRESS_USERNAME")]', process.env.WP_USERNAME);
      await slowInputFilling('//input[contains(@id, "WordPress App Password")]', process.env.WP_PASSWORD);
      break;

    case "gitlab":
      await slowInputFilling(Selectors.MCP.gitlabMCP.gitlabToken, process.env.GITLAB_TOKEN);
      break;

    case "bigquery":
      await slowInputFilling(Selectors.MCP.bigQuery.googleCloudProjectID, process.env.BQ_PROJECTID);
      await $(Selectors.MCP.bigQuery.googleCloudCredentials).setValue(process.env.BQ_APP_CREDS);
      break;

    case "datadog":
      await slowInputFilling(Selectors.MCP.datadog.datadogAPIKey, process.env.DD_API_KEY);
      await slowInputFilling(Selectors.MCP.datadog.datadogAPPKey, process.env.DD_APP_KEY);
      break;

    case "databricks utility":
      await slowInputFilling(Selectors.MCP.databricks.utility.workspaceHostname, process.env.DB_UTILITY_WORKSPACENAME);
      await slowInputFilling(Selectors.MCP.databricks.utility.functionCatalog, process.env.DB_UTILITY_FUNCATALOG || "workspace");
      await slowInputFilling(Selectors.MCP.databricks.utility.functionalSchema, process.env.DB_UTILITY_FUNSCHEMA || "information_schema");
      await slowInputFilling(Selectors.MCP.databricks.utility.PAT, process.env.DB_UTILITY_PAT);
      break;

    case "databricks vector space":
      await slowInputFilling(Selectors.MCP.databricks.utility.workspaceHostname, process.env.DB_UTILITY_WORKSPACENAME);
      await slowInputFilling(Selectors.MCP.databricks.vector.vectorCatalog, process.env.DB_UTILITY_FUNCATALOG || "workspace");
      await slowInputFilling(Selectors.MCP.databricks.vector.vectorSchema, process.env.DB_UTILITY_FUNSCHEMA || "information_schema");
      await slowInputFilling(Selectors.MCP.databricks.utility.PAT, process.env.DB_UTILITY_PAT);
      break;

    case "databricks genie space":
      await slowInputFilling(Selectors.MCP.databricks.utility.workspaceHostname, process.env.DB_UTILITY_WORKSPACENAME);
      await slowInputFilling(Selectors.MCP.databricks.genie.genieSpaceID, process.env.DB_GENIE_ID);
      await slowInputFilling(Selectors.MCP.databricks.utility.PAT, process.env.DB_UTILITY_PAT);
      break;

    case "brave search":
      await slowInputFilling(Selectors.MCP.brave.braveAPIKey, process.env.BRAVE_API_KEY);
      break;

    case "chroma cloud":
      await slowInputFilling(Selectors.MCP.chromaCloud.tenentID, process.env.CHROMA_TENENT_ID);
      await slowInputFilling(Selectors.MCP.chromaCloud.DBName, process.env.CHROMA_DB_NAME || "obot-test");
      await slowInputFilling(Selectors.MCP.chromaCloud.APIKey, process.env.CHROMA_API_KEY);
      break;

    case "firecrawl":
      await slowInputFilling(Selectors.MCP.fireCrawl.API_key, process.env.FIRECRAWL_API_KEY);
      break;

    case "gitmcp":
      await slowInputFilling(Selectors.MCP.gitMCP.urlLink, "https://gitmcp.io/docs");
      break;

    case "redis":
      await slowInputFilling(Selectors.MCP.redis.urlLink, process.env.REDIS_URL);
      break;

    case "tavily search":
      await slowInputFilling(Selectors.MCP.tavily.apiKeyInput, process.env.TAVILY_API_KEY);
      break;

    case "exa search":
      await slowInputFilling(Selectors.MCP.exa.apiKeyInput, process.env.TAVILY_API_KEY);
      break;

    case "pagerduty":
      await slowInputFilling(Selectors.MCP.pagerduty.apiKeyInput, process.env.PAGERDUTY_API_KEY);
      break;

    case "postman":
      await slowInputFilling(Selectors.MCP.postman.hostURL, "https://mcp.postman.com");
      await slowInputFilling(Selectors.MCP.postman.toolCOnfig, "mcp");
      await slowInputFilling(Selectors.MCP.postman.postmanAPIKey, process.env.POSTMAN_API_KEY);
      break;

    case "google cloud run":
      await $(Selectors.MCP.googleCloudRun.googleCloudCredentials).setValue(process.env.GCLOUD_APP_CREDS);
      break;

    case "grafana":
      await slowInputFilling(Selectors.MCP.grafana.grafanaURL, process.env.GRAFANA_URL);
      break;

    case "aws api":
    case "aws cdk":
    case "aws documentation":
    case "aws eks":
    case "aws kendra":
    case "aws redshift":
      await slowInputFilling(Selectors.MCP.aws.accessKeyIdInput, process.env.AWS_KEY_ID);
      await slowInputFilling(Selectors.MCP.aws.secretAccessKeyInput, process.env.AWS_ACCESS_ID);
      break;

    case "salesforce":
      await slowInputFilling(Selectors.MCP.salesforce.salesforceClientID, process.env.SF_CLIENT_ID);
      await slowInputFilling(Selectors.MCP.salesforce.salesforceClientSecret, process.env.SF_CLIENT_SECRET);
      break;

    case "slack":
      await slowInputFilling(Selectors.MCP.slack.slackBotToken, process.env.SLACK_BOT_TOKEN);
      break;

    case "ref":
      await slowInputFilling(Selectors.MCP.ref.refAPIKey, process.env.REF_API_KEY);
      break;
    
    case "render":
      await slowInputFilling(Selectors.MCP.render.renderAPIKey, process.env.RENDER_API_KEY);
      break;

    case "deepwiki":
    case "context7":
    case "antv charts":
      break;

    case "duckduckgo search":
    case "markitdown":
    case "microsoft learn":
    case "aws knowledge":
      switchLaunch = true;
      break;

    default:
      throw new Error(`Unknown MCP Server: ${mcpServer}`);
  }

  if (!switchLaunch) {
    await clickToElement(Selectors.MCP.btnClick("Launch"));
  }
  await browser.pause(LONG_PAUSE * 2);
});

When("User creates a new Project with no existing connections", async () => {
  await browser.pause(MEDIUM_PAUSE);
  const connectionElement = await $(Selectors.connectionsList).isDisplayed();
  const projectName = generateProjectName();
  if (connectionElement) {
    await clickToElement(Selectors.currentProjectButton);
    await clickToElement(Selectors.createNewProjectButton);
    await slowInputFilling(Selectors.inputNewProjectName, projectName);
    await clickToElement(Selectors.saveButton);
  }
  await browser.pause(MEDIUM_PAUSE);
  await browser.refresh();
});

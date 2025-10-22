import { When, Then, Given } from "@wdio/cucumber-framework";
import Selectors from "../core/selectors";
import { clickToElement,isElementDisplayed,slowInputFilling} from "../core/func";
import { LONG_PAUSE, SHORT_PAUSE } from "../core/timeouts";
import { aggregateToolResponses, saveMCPReport, sendPromptValidateAndCollect } from "../core/mcpFunc";

let responses: string[] = [];
const wordpressTools = [
  "create_category", "create_post", "create_tag",
  "delete_category", "delete_media", "delete_post", "delete_tag",
  "get_me", "get_site_settings",
  "list_categories", "list_media", "list_posts", "list_tags", "list_users",
  "retrieve_post", "update_category", "update_media", "update_post", "update_tag",
  "validate_credential"
];

const gitlabTools = [
  "search_repositories",
  "create_repository",
  "get_project",
  "list_projects",
  "get_file_contents",
  "create_or_update_file",
  "push_files",
  "get_repository_tree",
  "create_branch",
  "create_issue",
  "list_issues",
  "update_issue",
  "create_merge_request",
  "list_merge_requests",
  "get_merge_request",
  "merge_merge_request",
  "update_merge_request",
  "create_merge_request_thread",
  "fork_repository",
  "get_users"
];

Given(/^User navigates the Obot main login page$/, async() => {
	const url = process.env.mainURL ; 
	await browser.url(url);
});

Then(/^User open chat Obot$/, async () => {
	await clickToElement(Selectors.MCP.navigationbtn);
	await clickToElement(Selectors.MCP.clickChatObot);
});

When(/^User open MCP connector page$/, async () => {
	await clickToElement(Selectors.MCP.connectorbtn);
});

Then(/^User select "([^"]*)" MCP server$/, async (MCPServer) => {
	await slowInputFilling(Selectors.MCP.mcpSearchInput, MCPServer);
	await isElementDisplayed(Selectors.MCP.selectMCPServer(MCPServer), LONG_PAUSE);
	// Wait until matching elements appear
	const allServers = await $$(Selectors.MCP.selectMCPServer(MCPServer));
	if (allServers.length === 0) throw new Error(`No MCP server found matching: ${MCPServer}`);

	// Click the last one
	const lastServer = allServers[allServers.length - 1];
	await lastServer.waitForDisplayed({ timeout: LONG_PAUSE });
	await lastServer.click();

	await browser.pause(SHORT_PAUSE);
});

Then(/^User select "([^"]*)" button$/, async (Button) => {
	await isElementDisplayed(Selectors.MCP.btnClick(Button),SHORT_PAUSE);
	await clickToElement(Selectors.MCP.btnClick(Button));
});

Then(/^User connect to the WordPress1 MCP server$/, async () => {
	await slowInputFilling(Selectors.MCP.wpSiteURL,process.env.WPURL);
	await slowInputFilling(Selectors.MCP.wpUsername,process.env.WPUsername);
	await slowInputFilling(Selectors.MCP.wpPassword, process.env.WPPassword);
	await clickToElement(Selectors.MCP.btnClick("Launch"));
	await browser.pause(LONG_PAUSE*2);
});
		
Then(/^User asks obot "([^"]*)"$/, async (prompt) => {
	await slowInputFilling(Selectors.MCP.obotInput, prompt);
	await clickToElement(Selectors.MCP.submitPrompt);
	await browser.pause(LONG_PAUSE);
});

Then(/^User connect to the GitLab MCP server$/, async () => {
	await slowInputFilling(Selectors.MCP.gitlabToken,process.env.gitLabToken);
	await clickToElement(Selectors.MCP.btnClick("Launch"));
	await browser.pause(LONG_PAUSE);
});

When(/^User sends following prompts to Obot AI chat for "([^"]*)" MCP server:$/, { timeout: 15 * 60 * 1000 }, async function(serverName: string, table) {
  const prompts = table.raw().slice(1).map((row: any[]) => row[0]);
  this.promptResults = [];
  let toolList;

  toolList =`${serverName.toLowerCase()}Tools`;

  for (let i = 0; i < prompts.length; i++) {
    try {
      const result = await sendPromptValidateAndCollect(prompts[i], toolList, i);
      this.promptResults.push(result);
    } catch (err) {
      console.error(`Error in prompt #${i+1}: ${err.message}`);
      this.promptResults.push({ prompt: prompts[i], error: err.message });
    }
  }
});

Then(/^All prompts results should be validated and report generated for selected "([^"]*)" MCP Server$/, async function(serverName: string) {
  const report = aggregateToolResponses(this.promptResults);
  saveMCPReport(serverName, report);

  const errors = this.promptResults.filter(r => r.error);
  if (errors.length > 0) {
    console.warn(`${errors.length} prompts had issues.`);
  }
});


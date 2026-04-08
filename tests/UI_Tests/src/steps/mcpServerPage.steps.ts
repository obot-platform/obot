import { When, Then } from '@wdio/cucumber-framework';
import Selectors from "../core/selectors";
import {
    clickToElement,
  slowInputFilling,
} from "../core/func";
import { Key } from "webdriverio";
import { expect } from '@wdio/globals';
import { MEDIUM_PAUSE, WAIT_FOR_TIMEOUT, SHORT_TIMEOUT } from '../core/timeouts';

When(/^User searches for MCP server "([^"]*)"$/, async (serverName: string) => {
    await browser.pause(MEDIUM_PAUSE);
    if(await $(Selectors.MCP.adminNavigation.welcomeToObotTitle).isDisplayed()) {
        await clickToElement(Selectors.MCP.popupContinueButton);
    }
    const input = await $(Selectors.MCP.serversPage.searchInput);
    await input.waitForDisplayed();
    await input.clearValue();
    await input.setValue(serverName);
    await browser.pause(MEDIUM_PAUSE);
});

When(/^User performs "([^"]*)" action on MCP server "([^"]*)"$/, async (action: string, serverName: string) => {
    const actionMenu = await $(Selectors.MCP.serversPage.actionMenu(serverName));

    await actionMenu.waitForExist({ timeout: 10000 });
    await actionMenu.scrollIntoView();
    await browser.pause(200); // allow layout to settle

    await actionMenu.waitForDisplayed();
    await actionMenu.waitForEnabled();

    // move mouse before clicking
    await actionMenu.moveTo();
    await browser.pause(100);

    await actionMenu.click();

    await $(Selectors.MCP.serversPage.menuActionBtn(action)).waitForDisplayed();
    await $(Selectors.MCP.serversPage.menuActionBtn(action)).click();
});

When(/^User updates MCP server config for "([^"]*)" and saves$/, async (serverName: string) => {
    switch (serverName.toLowerCase()) {

        case "gitlab":
        case "brave search":
        case "aws api":
            break;

        case "antv charts":
            await slowInputFilling(Selectors.MCP.antvCharts.requestServerInput, '1111');
            await slowInputFilling(Selectors.MCP.antvCharts.disabledToolsInput, '2222');
            await slowInputFilling(Selectors.MCP.antvCharts.serviceIdInput, '3333');
            break;

        default:
            throw new Error(`Unknown MCP Server: ${serverName}`);
    }
    await $(Selectors.MCP.serversPage.updateConfigBtn).click();
});

Then(/^MCP server config should be updated for "([^"]*)"$/, async (serverName: string) => {
    switch (serverName.toLowerCase()) {

        case "gitlab":
        case "brave search":
        case "aws api":
            break;

        case "antv charts":
            const requestServerVal = await $(Selectors.MCP.antvCharts.requestServerInput).getValue();
            await expect(requestServerVal).toBe('1111');
            const disabledToolsVal = await $(Selectors.MCP.antvCharts.disabledToolsInput).getValue();
            await expect(disabledToolsVal).toBe('2222');
            const serviceIdVal = await $(Selectors.MCP.antvCharts.serviceIdInput).getValue();
            await expect(serviceIdVal).toBe('3333');
            break;

        default:
            throw new Error(`Unknown MCP Server: ${serverName}`);
    }
    await browser.keys(Key.Escape);
});

When(/^User updates MCP server name to "([^"]*)"$/, async (newName: string) => {
    const input = await $(Selectors.MCP.serversPage.renameInput);
    await input.waitForDisplayed();
    await input.setValue(newName);
    await $(Selectors.MCP.serversPage.renameUpdateBtn).click();
});

Then(/^MCP server name should be updated to "([^"]*)" on details page$/, async (expectedName: string) => {
    await expect($(Selectors.MCP.serversPage.serverNameInDetailsHeader)).toHaveText(expectedName);
});

Then(/^MCP server details for "([^"]*)" should be correct$/, async (serverName: string) => {
    await expect($(Selectors.MCP.serversPage.serverNameInDetails)).toHaveText(serverName);
    switch (serverName.toLowerCase()) {

        case "gitlab":
            await expect($(Selectors.MCP.serversPage.gitlabDescription)).toHaveText(
                'A Model Context Protocol (MCP) server that provides comprehensive GitLab integration capabilities. Enable LLMs to interact with GitLab projects, manage repositories, handle issues and merge requests, and perform automated GitLab operations with precision.'
            );
            break;

        case "brave search":
            await expect($(Selectors.MCP.serversPage.braveSearchDescription)).toHaveText(
                'A Model Context Protocol (MCP) server that provides comprehensive web search capabilities through the Brave Search API. It enables AI assistants to search the web, find news articles, discover videos and images, locate local businesses, and generate AI-powered summaries of search results.'
            );
            break;

        case "aws api":
            await expect($(Selectors.MCP.serversPage.awsApiDescription)).toHaveText(
                'A Model Context Protocol (MCP) server for AWS CLI operations. Execute AWS CLI commands and get intelligent command suggestions across all AWS services safely and efficiently.'
            );
            break;

        case "antv charts":
            await expect($(Selectors.MCP.serversPage.antvChartsDescription)).toHaveText(
                'A Model Context Protocol (MCP) server for generating charts using AntV. Create various types of professional charts for data analysis and visualization with support for 25+ chart types including statistical, geographical, and specialized diagrams.'
            );
            break;

        default:
            throw new Error(`Unknown MCP Server: ${serverName}`);
    }
});

When(/^User navigates back to MCP server list$/, async () => {
    const backButton = await $(Selectors.MCP.serversPage.backBtn);
    await backButton.waitForDisplayed({ timeout: SHORT_TIMEOUT });
    await backButton.click();
    // Wait for page to navigate back
    await $(Selectors.MCP.serversPage.searchInput).waitForDisplayed({ 
        timeout: WAIT_FOR_TIMEOUT 
    });
});

When(/^User goes back to MCP server list$/, async () => {
    await browser.url(`${process.env.OBOT_URL}/admin/mcp-servers`);
    // Wait for the page to load and search input to be visible
    await $(Selectors.MCP.serversPage.searchInput).waitForDisplayed({ 
        timeout: WAIT_FOR_TIMEOUT 
    });
});

Then(/^MCP server "([^"]*)" should be disconnected successfully$/, async (serverName: string) => {
    await expect($(Selectors.MCP.serversPage.connectedStatus(serverName))).not.toBeDisplayed();
});

Then(/^User closes the MCP server connection modal$/, async () => {
    await $(Selectors.MCP.adminNavigation.connectionUrlLabel).waitForDisplayed({ timeout: WAIT_FOR_TIMEOUT });
    await browser.keys(Key.Escape);
});
import {When, Then } from "@wdio/cucumber-framework";
import { clickToElement, slowInputFilling } from "../core/func";
import { LONG_PAUSE, MEDIUM_PAUSE } from "../core/timeouts";
import { expect } from '@wdio/globals';
import Selectors from "../core/selectors";

When(/^User clicks on "Add MCP Server" button$/, async() => {
    await browser.pause(MEDIUM_PAUSE);
    if(await $(Selectors.MCP.adminNavigation.welcomeToObotTitle).isDisplayed()) {
        await clickToElement(Selectors.MCP.popupContinueButton);
    }
    await clickToElement(Selectors.MCP.compositeServers.addMcpServerButton);
    await browser.pause(MEDIUM_PAUSE);
    await clickToElement(Selectors.MCP.compositeServers.addServerMenuButton);
});

When(/^User selects "([^"]*)" option from select server type modal$/, async(serverType: string) => {
    await clickToElement(Selectors.MCP.compositeServers.selectServerTypeButton(serverType));
    await browser.pause(MEDIUM_PAUSE);
});

When(/^User enters "([^"]*)" in MCP server name field$/, async(serverName: string) => {
    await slowInputFilling(Selectors.MCP.compositeServers.serverNameInput, serverName);
    await browser.pause(MEDIUM_PAUSE);
});

When(/^User add "([^"]*)" component servers to composite server$/, async(componentServers: string) => {
    const servers = componentServers.split(',');
    for (const server of servers) {
        const trimmed = server.trim();
        await clickToElement(Selectors.MCP.compositeServers.addComponentServerButton);
        // select component servers from the modal
        await slowInputFilling(Selectors.MCP.compositeServers.componentSearchInput, trimmed);
        await browser.pause(MEDIUM_PAUSE);
        await clickToElement(Selectors.MCP.compositeServers.componentServerOption(trimmed));
        // wait for configure tools modal and click skip
        await $(Selectors.MCP.compositeServers.configureToolsDialogTitle(trimmed)).waitForDisplayed();
        await clickToElement(Selectors.MCP.compositeServers.skipConfigureButton);
        // validate component server is added
        await expect($(Selectors.MCP.compositeServers.componentServerInList(trimmed))).toBeDisplayed();
    }
});

When(/^User save the composite MCP server$/, async() => {
    await clickToElement(Selectors.MCP.compositeServers.saveButton);
    await browser.pause(LONG_PAUSE);
});

When(/^User skip connect to "([^"]*)" MCP server$/, async(connectionName: string) => {
    await $(Selectors.MCP.compositeServers.skipConnectDialogTitle(connectionName)).waitForDisplayed();
    await clickToElement(Selectors.MCP.compositeServers.skipConnectButton);
});

Then(/^MCP server "([^"]*)" should be added successfully$/, async(serverName: string) => {
    const createCompositeServerHeader = await $(Selectors.MCP.compositeServers.createCompositeServerHeader);
    if (await createCompositeServerHeader.isDisplayed()) {
        await clickToElement(Selectors.MCP.serversPage.backBtn);
        await browser.pause(MEDIUM_PAUSE);
    }
    const serverRow = await $(Selectors.MCP.compositeServers.serverRow(serverName));
    await serverRow.waitForDisplayed({ timeout: LONG_PAUSE });
    await expect(serverRow).toBeDisplayed();
});

Then(/^User Connect to "([^"]*)" MCP server$/, async(serverName: string) => {
    await $(Selectors.MCP.compositeServers.compositeServerDialogTitle).waitForDisplayed();
    await clickToElement(Selectors.MCP.compositeServers.continueButton);

    switch (serverName) {
        case 'AntV Exa Composite Server':
            // add Exa API key
            await slowInputFilling(Selectors.MCP.exa.apiKeyInput, process.env.EXA_API_KEY);
            break;
        case 'AWS Composite Server': {
            // add AWS Access Key and Secret Key
            const awsKeyInputs = await $$(Selectors.MCP.aws.accessKeyIdInput);
            for (const input of awsKeyInputs) {
                await input.setValue(process.env.AWS_KEY_ID);
            }
            const awsSecretInputs = await $$(Selectors.MCP.aws.secretAccessKeyInput);
            for (const input of awsSecretInputs) {
                await input.setValue(process.env.AWS_ACCESS_ID);
            }
            const awsRegionInputs = await $$(Selectors.MCP.aws.regionInput);
            for (const input of awsRegionInputs) {
                await input.setValue('us-east-2');
            }
            await slowInputFilling(Selectors.MCP.aws.genericAccessKeyIdInput, process.env.AWS_KEY_ID);
            break;
        }
        case 'Gitlab Wordpress Composite Server':
            // add GitLab Personal Access Token
            await slowInputFilling(Selectors.MCP.gitlabMCP.gitlabToken, process.env.GITLAB_TOKEN);
            // add WordPress Admin Password
            await slowInputFilling('//input[contains(@id, "WORDPRESS_SITE")]', process.env.WP_URL);
            await slowInputFilling('//input[contains(@id, "WORDPRESS_USERNAME")]', process.env.WP_USERNAME);
            await slowInputFilling('//input[contains(@id, "WordPress App Password")]', process.env.WP_PASSWORD);
            break;
        default:
            throw new Error(`Unknown MCP Server: ${serverName}`);
    }
    
    await clickToElement(Selectors.MCP.btnClick("Launch"));
});
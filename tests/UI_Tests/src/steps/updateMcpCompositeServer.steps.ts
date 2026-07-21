import {When, Then } from "@wdio/cucumber-framework";
import { clickToElement } from "../core/func";
import { MEDIUM_PAUSE } from "../core/timeouts";
import { expect } from '@wdio/globals';
import Selectors from "../core/selectors";
import { Key } from "webdriverio";

When(/^User open "([^"]*)" tab$/, async(tabName: string) => {
    await clickToElement(Selectors.MCP.compositeServers.serverTab(tabName));
});

When(/^User delete existing component server "([^"]*)" from composite MCP server$/, async(componentServer: string) => {
    await clickToElement(Selectors.MCP.compositeServers.componentServerInListDeleteButton(componentServer));
    // validate component server is removed
    await expect($(Selectors.MCP.compositeServers.componentServerInList(componentServer))).not.toBeDisplayed();

    await clickToElement(Selectors.MCP.compositeServers.updateCompositeServerButton);
});

Then(/^"([^"]*)" component servers of "([^"]*)" composite MCP server should be displayed$/, async(componentServers: string, compositeServerName: string) => {
    const servers = componentServers.split(',');
    for (const server of servers) {
        const trimmed = server.trim();
        await expect($(`//td//p[normalize-space(text()) = "${trimmed}" and .//span[normalize-space(.) = "(${compositeServerName})"]]`)).toBeDisplayed();
    }
});

When(/^User confirms update of composite MCP server$/, async() => {
    await $(`//h4[starts-with(normalize-space(.), "Update")]`).waitForDisplayed();
    await clickToElement(`//dialog[@open]//button[normalize-space()="Yes, I'm sure"]`);
});

Then(/^"([^"]*)" component server should be deleted from "([^"]*)" composite MCP server$/, async(deletedServer: string, compositeServerName: string) => {
    await expect($(`//td//p[normalize-space(text())="${deletedServer}" and .//span[normalize-space(.)="(${compositeServerName})"]]`)).not.toBeDisplayed();
});

Then(/^Configuration diff modal should be displayed for "([^"]*)" composite MCP server$/, async(compositeServerName: string) => {
    const diffModal = await $(`//dialog[@open]//h3//div[contains(text(), "${compositeServerName}")]`);
    await diffModal.waitForDisplayed({ timeout: MEDIUM_PAUSE });
    await expect(diffModal).toBeDisplayed();
    const currentVerTxt = await $('//dialog[@open]//h3[text()="Current Version"]');
    await expect(currentVerTxt).toBeDisplayed();
    const newVerTxt = await $('//dialog[@open]//h3[text()="New Version"]');
    await expect(newVerTxt).toBeDisplayed();
    await browser.keys(Key.Escape);
});

Then(/^"([^"]*)" action on MCP server "([^"]*)" should not be available$/, async(actionOptions: string, compositeServerName: string) => {
    const actionMenu = await $(Selectors.MCP.serversPage.actionMenu(compositeServerName));
    await actionMenu.waitForExist({ timeout: MEDIUM_PAUSE });
    await actionMenu.scrollIntoView();
    await browser.pause(200); // allow layout to settle

    await actionMenu.waitForDisplayed();
    await actionMenu.waitForEnabled();

    // move mouse before clicking
    await actionMenu.moveTo();
    await browser.pause(100);

    await actionMenu.click();

    for (const action of actionOptions.split(',')) {
        const menuBtn = await $(Selectors.MCP.serversPage.menuActionBtn(action.trim()));
        await expect(menuBtn).not.toBeDisplayed();
    }
});


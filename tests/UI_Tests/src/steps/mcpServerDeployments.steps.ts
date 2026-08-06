import { When, Then } from '@wdio/cucumber-framework';
import { clickToElement } from "../core/func";
import { expect } from '@wdio/globals';
import Selectors from "../core/selectors";

When(/^User navigate to deployments and connections tab$/, async () => {
    await clickToElement(Selectors.MCP.deployments.deploymentsAndConnectionsTabButton);
  }
);

Then(/^"([^"]*)" MCP server should be restarted successfully$/, async (serverName: string) => {
  const lastRestartValue = await $(Selectors.MCP.deployments.lastRestartValue);

  const lastRestartText = await lastRestartValue.getText();

  if (lastRestartText === 'just now') {
      expect(true).toBe(true);
  } else {
      const seconds = parseInt(lastRestartText.match(/\d+/)[0], 10);
      await expect(seconds).toBeLessThan(59);
  }
});

When(/^User confirms deletion of MCP server "([^"]*)"$/, async (serverName: string) => {
  const deleteConfirm = await $(Selectors.MCP.deployments.deleteConfirmDialog(serverName));
  await deleteConfirm.waitForDisplayed();
  
  const confirmDeleteButton = await $(Selectors.MCP.deployments.confirmDeleteButton(serverName));
  await confirmDeleteButton.click();
});

Then(/^MCP server "([^"]*)" should be deleted successfully$/, async (serverName: string) => {
  const serverRow = await $(Selectors.MCP.deployments.deletedServerRow(serverName));
  await expect(serverRow).not.toExist();

  // go back to MCP server list
  await $(Selectors.MCP.deployments.serverEntriesButton).click();
  const input = await $(Selectors.MCP.serversPage.searchInput);
  await input.waitForDisplayed();
  await input.setValue(serverName);

  await expect($(Selectors.MCP.serversPage.connectedStatus(serverName))).not.toBeDisplayed();
});
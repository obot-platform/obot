import {When, Then } from "@wdio/cucumber-framework";
import { clickToElement, slowInputFilling } from "../core/func";
import { LONG_PAUSE, MEDIUM_PAUSE } from "../core/timeouts";
import { expect } from '@wdio/globals';
import Selectors from "../core/selectors";

When(/^User clicks on "Start New Task" button$/, async() => {
    await clickToElement('//p[normalize-space()="Tasks"]/following-sibling::button');
});

When(/^User enters "([^"]*)" in Task name field$/, async(taskName: string) => {
    await slowInputFilling('//strong[normalize-space()="TASK"]/following-sibling::input[1]', taskName);
});

When(/^User enters description in Task description field$/, async() => {
    await slowInputFilling('//input[@placeholder="Enter description..."]', 'Sum of two numbers and square of the sum');
});

When(/^User selects "([^"]*)" option from Task type dropdown$/, async(taskType: string) => {
    const dropdownTrigger = await $(
        '//h3[text()="How do you want to run this task?"]/following-sibling::button[normalize-space()="on interval" or normalize-space()="on demand"]'
    );
    await dropdownTrigger.click();

    const option = await $(
        `//li/button[normalize-space()="${taskType.toLowerCase()}"]`
    );
    await option.waitForClickable({ timeout: LONG_PAUSE });
    await option.click();
});

When(/^User adds an arguments to the On Demand task$/, async() => {
    const data = [
      { name: 'x', description: 'value of x' },
      { name: 'y', description: 'value of y' }
    ];
    // add arguments
    for (let i = 0; i < data.length; i++) {
        await clickToElement('//button[normalize-space()="Argument"]');
        const arg = data[i];
        const rows = await $$('//tbody//tr[contains(@class,"group")]');
        const nameInput = await rows[i].$('input[placeholder="Enter Name"]');
        const descInput = await rows[i].$('textarea[placeholder="Add a good description"]');

        await nameInput.waitForDisplayed();
        await nameInput.setValue(data[i].name);

        await descInput.waitForDisplayed();
        await descInput.setValue(data[i].description);
    }
    await browser.pause(MEDIUM_PAUSE);
});

When(/^User adds steps to the On Demand task$/, async() => {
    const steps = [
      'Sum of x and y',
      'Square of the sum',
    ];

    // get initial rows
    let rows = await $$(
      '//ol[contains(@class,"draggable-list")]//li[contains(@class,"draggable-element")]'
    );

    for (let i = 0; i < steps.length; i++) {
        const currentRow = rows[i];

        const textarea = await currentRow.$(
            'textarea[placeholder="Instructions..."]'
        );
        await textarea.waitForDisplayed();
        await textarea.setValue(steps[i]);

        // click add step except for last iteration
        if (i < steps.length - 1) {
            const addBtn = await currentRow.$(
                'button[data-testid="step-add-btn"]'
            );
            await addBtn.click();

            // wait for new row to appear
            await browser.waitUntil(async () => {
                rows = await $$(
                  '//ol[contains(@class,"draggable-list")]//li[contains(@class,"draggable-element")]'
                );
                return rows.length === i + 2;
            }, { timeout: 5000 });
        }
    }
});

When(/^User clicks on "Run" button to run the On Demand task$/, async() => {
    await clickToElement('//strong[normalize-space()="TASK"]/ancestor::div[contains(@class,"mt-8")]//button[.//text()[contains(., "Run")]]');
});

When(/^User inputs arguments for the On Demand task$/, async() => {
  await $('//dialog[@open]//h3[normalize-space()="Run Task"]').waitForDisplayed({ timeout: MEDIUM_PAUSE });
  await slowInputFilling('//input[@id="param-x"]', '5');
  await slowInputFilling('//input[@id="param-y"]', '10');
});

When(/^User clicks on "Run" button in Run Task dialog$/, async() => {
    await clickToElement('//dialog[@open]//button[normalize-space()="Run"]');
    await browser.pause(LONG_PAUSE);
});

Then(/^Validate Task results for the On Demand task$/, async() => {
  const expectedResults = ['15', '225'];

  // Wait for results to appear
  await browser.pause(MEDIUM_PAUSE);
  let responses = await $$('//div[@class="message-content"]');
  await browser.waitUntil(async () => {
    responses = await $$('//div[@class="message-content"]');
    return await responses.length >= 2;
  }, { timeout: 30000, interval: 1000, timeoutMsg: 'Expected at least 2 result messages to appear' });

  // Extract and validate outputs
  await browser.pause(MEDIUM_PAUSE); // allow any final rendering
  const outputTexts = await Promise.all(
    await responses.map(async elem => await elem.getText())
  );
  console.log('Task output texts:', outputTexts);

  for (let i = 0; i < expectedResults.length; i++) {
    expect(outputTexts[i]).toContain(expectedResults[i]);
  }
});

When(/^User clicks on "Delete Task" button to delete the On Demand task$/, async() => {
    await clickToElement('//strong[normalize-space()="TASK"]/ancestor::div[contains(@class,"mt-8")]//button[contains(@class, "button-destructive")]');
    await browser.pause(MEDIUM_PAUSE);
    await $('//dialog[@open]//h3[text()="Are you sure you want to delete this task?"]').waitForDisplayed();
    await clickToElement(`//dialog[@open]//button[normalize-space(text())="Yes, I'm sure"]`);
});

Then(/^On Demand task should be deleted successfully$/, async() => {
    await browser.waitUntil(async () => {
      const element = await $('//p[text()="Tasks"]/parent::div/following-sibling::ul/li//button[text()="Sum of two numbers and square"]');
      return !(await element.isDisplayed());
    }, { timeout: LONG_PAUSE });

    expect(await $('//p[text()="Tasks"]/parent::div/following-sibling::ul/li//button[text()="Sum of two numbers and square"]').isDisplayed()).toBe(false);
});

When(/^User clicks on "Toggle Chat" button to open Obot chat panel$/, async() => {
    await clickToElement('//strong[normalize-space()="TASK"]/ancestor::div[contains(@class,"mt-8")]//button[contains(@class, "icon-button")]');
    await browser.pause(MEDIUM_PAUSE);
});

Then(/^Obot chat panel should be opened successfully$/, async() => {
    const chatPanel = await $('//div[@id="chat"]');
    await chatPanel.waitForDisplayed({ timeout: LONG_PAUSE });
    expect(await chatPanel.isDisplayed()).toBe(true);
});


When(/^User sends prompt "([^"]*)" to Obot in Task chat panel$/, async (promptText: string) => {
    const input = await $('//div[@id="chat"]//div[@role="textbox"]');
    await input.waitForDisplayed({ timeout: LONG_PAUSE });
    await input.addValue(promptText);

    await clickToElement(Selectors.MCP.submitPrompt);
    await browser.pause(LONG_PAUSE);
});

Then(/^Validate that Obot responds with a result$/, async () => {
  // Wait for results to appear
  await browser.pause(MEDIUM_PAUSE);
  let responses = await $$('//div[@class="message-content"]');
  await browser.waitUntil(async () => {
    responses = await $$('//div[@class="message-content"]');
    return await responses.length >= 3;
  }, { timeout: 30000, interval: 1000, timeoutMsg: 'Expected at least 3 result messages to appear' });

  // Extract and validate outputs
  await browser.pause(LONG_PAUSE); // allow any final rendering
  const outputTexts = await Promise.all(
    await responses.map(async elem => await elem.getText())
  );

  expect(outputTexts[outputTexts.length - 1]).toContain("summary");
});
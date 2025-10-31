import { Given, When, Then } from "@wdio/cucumber-framework";
import Selectors from "../core/selectors";
import { clickToElement, element, slowInputFilling } from "../core/func";
import { LONG_PAUSE, MEDIUM_PAUSE } from "../core/timeouts";
import SoftAssert from "../core/softAssert";
import { context } from "../utils/context";
import { Key } from "webdriverio";

Given(/^User navigates the Obot login page$/, async() => {
	const url = process.env.baseURL ; // Fallback URL if not defined
	await browser.url(url);
});

When(/^User clicks on the Login button$/, async() => {
	await clickToElement(Selectors.loginButton)
});

When(/^User selects Okta to sign in$/, async() => {
	await clickToElement(Selectors.continueWithOkta)
});

When(/^User selects Google to sign in$/, async() => {
	await clickToElement(Selectors.continueWithGoogle)
});

When(/^User enters valid google credentials and logs in$/, async() => {
	// await clickToElement(Selectors.clickonAccount)
	(await element(Selectors.emailId)).setValue(process.env.userName)
	await browser.pause(5000)
    await slowInputFilling(Selectors.passwordField,process.env.passWord)
	await clickToElement(Selectors.submitButton)
});

When(/^User enters valid credentials and logs in$/, async() => {
    await slowInputFilling(Selectors.usernameField,process.env.user_Name)
    await slowInputFilling(Selectors.passwordField,process.env.passWord)
	await browser.pause(MEDIUM_PAUSE)
	await clickToElement(Selectors.submitButton)
	await browser.pause(MEDIUM_PAUSE)
});

Then('I setup context for assertion', async () => {
    context().softAssert = new SoftAssert();
});

Then('ui: I check that all assertions are correct', async () => {
    await context().softAssert.assertAll();
});

Then('ui: Press Escape Key', async () => {
	await browser.keys(Key.Escape);
  });


When(/^User navigates the Obot-Admin login page$/, async() => {
	const url = process.env.adminURL ; 
	await browser.url(url);
});

Then(/^User selects Okta to sign in Obot-Admin$/, async() => {
	await clickToElement(Selectors.admin.oktaLogin)
});

Then(/^User navigate to created agent$/, async() => {
	const url = process.env.adminURL ; 
	await browser.url(url);
	await browser.pause(LONG_PAUSE)
});

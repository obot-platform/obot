/* eslint-disable import/no-extraneous-dependencies */
/* eslint-disable @typescript-eslint/no-unused-vars */

import { SHORT_PAUSE, ELEMENT_TIMEOUT, SHORT_TIMEOUT } from './timeouts';
import { IFormActionElements } from './types';
// import cheerio from 'cheerio';

export async function element(locatorString: string) {
  try {
    const element = await $(locatorString);
    await element.waitForExist({ timeout: ELEMENT_TIMEOUT });
    return element;
  } catch (e) {
    console.error('error with element with name: ', locatorString);
    throw e;
  }
}

export async function mayBeElement(locatorString: string, timeout = ELEMENT_TIMEOUT) {
  try {
    const element = await $(locatorString);
    await element.waitForDisplayed({ timeout: timeout }); // only for this method
    return element;
  } catch (e) {
    return false;
  }
}

export function locatorParse(locatorString: string, parsing: string) {
  return locatorString.replaceAll(/{{\w+}}/ig, parsing);
}

export async function elementParse({ locatorString, parsing, options = {} }) {
  const _locatorString = locatorString.replace(/{{\w+}}/ig, parsing);
  const el = await $(_locatorString);
  if (await el.isExisting() || await el.waitForDisplayed({ timeout: ELEMENT_TIMEOUT })) {
    return el;
  } return undefined;
}

export async function clickToElement(locatorString: string, timeout: number = ELEMENT_TIMEOUT, waitForClickable = true) {
  const el = await element(locatorString);

  if (waitForClickable) {
    await el.waitForClickable({ timeout: timeout });
  } else {
    await browser.pause(SHORT_PAUSE);
  }

  await el.click();
}

export async function slowInputFilling(locatorString: string, value: any) {
  /* eslint-disable no-constant-condition */

  const el = await element(locatorString);
  for (let i = 0; i < 8; i++) {
    await el.waitForEnabled();
    if (await el.isClickable()) {
      await el.click();
      await clearTextUsingBackspace(el);
    } else {
      try {
        await el.clearValue();
      } catch (error) {
        if (error.message.includes("element not interactable")) {
          //Needed as sometimes elements are not interactable immediately
          await browser.pause(SHORT_PAUSE);
        } else {
          throw Error(error);
        }
      }
    }
    await el.setValue(value);
    await browser.pause(SHORT_PAUSE);
    const setValue = await el.getValue();
    if (setValue === value) {
      break;
    }

  }
}

export async function formFill(formElements: IFormActionElements[]) {
  const setValue = async (element: WebdriverIO.Element, action: string, value: any) => {
    await element.waitForEnabled();
    switch (action) {
      case 'click':
        await element.click();
        break;
      case 'clearValue':
        await element.click(); // To select field for clearValue()
        await browser.keys(['\uE009', 'a', 'Back space']);
        await element.clearValue();
        await browser.pause(SHORT_PAUSE);
        break;
      case 'hitEnter':
        await browser.keys(`\uE007`);
        break;
      case 'doubleClick':
        await element.doubleClick();
        break;
      default:
        await element.clearValue();
        await element[action](value);
        break;
    }
  };

  for (const { locator, action, value } of formElements) {
    try {
      const el = await element(locator);
      if (!el) { throw Error(`element can not located with path | location : ${locator}`); }
      await setValue(el, action, value);
    } catch (e) {
      console.error('error during fillForm: ', action, value, e);
      throw e;
    }
  }
}

export async function generateRandomAlphanumericString(length) {
  const characters = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
  let result = ' ';
  const charactersLength = characters.length;
  for (let i = 0; i < length; i++) {
    result += characters.charAt(Math.floor(Math.random() * charactersLength));
  }
  return result;
}

export async function elementParseAndClick(locatorString: string, parsing: string, waitForClickable = true) {
  const _locatorString = locatorString.replace(/{{\w+}}/ig, parsing);
  const el = await element(_locatorString);

  if (waitForClickable) {
    await el.waitForClickable({ timeout: ELEMENT_TIMEOUT });
  } else {
    await browser.pause(SHORT_PAUSE);
  }

  await el.click();
}

export async function reload() {
  const url = await browser.getUrl();
  await browser.url(url.toString());
  await expect(await browser.getUrl()).toEqual(url.toString());
}

export async function isElementSelected(element: WebdriverIO.Element) {
  if (await element.isSelected()) {
    return true;
  } else {
    return false;
  }
}

export async function doubleClickToElement(locatorString: string, waitForClickable = true) {
  const el = await element(locatorString);

  if (waitForClickable) {
    await el.waitForClickable({ timeout: ELEMENT_TIMEOUT });
  } else {
    await browser.pause(SHORT_PAUSE);
  }
  await el.doubleClick();
}

export async function clickToElementUsingJS(locatorString: string) {
  const el = await element(locatorString);
  await browser.pause(SHORT_PAUSE);
  await browser.execute("arguments[0].click();", el);
}

export async function isElementDisplayed(locatorString: string, timeout = SHORT_TIMEOUT) {
  const element = await mayBeElement(locatorString, timeout);
  if (element === false || element === undefined) {
    return false;
  } else {
    return true;
  }
}

export async function clearTextUsingBackspace(element: WebdriverIO.Element) {
  let text = await element.getText();
  if (text.length === 0) {
    text = await element.getValue();
  }
  await element.click();
  await browser.pause(SHORT_PAUSE / 2);
  await browser.keys(['Control', 'a']);
  await browser.keys('Backspace');
  await browser.keys(['Control', 'NULL']);
}

export async function elementGetText(locatorString: string) {
  const elementForFetchingText = await element(locatorString);
  return await elementForFetchingText.getText();
}

export async function elementGetValue(locatorString: string) {
  const elementForFetchingValue = await element(locatorString);
  return await elementForFetchingValue.getValue();
}

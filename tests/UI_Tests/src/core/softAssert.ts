import allure from '@wdio/allure-reporter'
import { Status } from 'allure-js-commons';
const chai = require('fix-esm').require('chai');
const assert = chai.assert;

class SoftAssert {
  private assertionErrorMessages: string[] = [];

  async equal(actual: any, expected: any, message: string) {
    try {
      assert.deepStrictEqual(actual, expected, message);
      allure.addStep(`Verify if Actual value:[${actual}] is equal to Expected value:[${expected}]`, Status.PASSED);
    } catch (error) {
      const screenshot = await browser.takeScreenshot();
      const pageSource = await browser.getPageSource();
      allure.addStep(`Verify if Actual value:[${actual}] is equal to Expected value:[${expected}]`,
        {
          attachments: [
            { name: 'Screenshot', content: screenshot, type: 'image/png' },
            { name: 'Page Source', content: pageSource, type: 'text/html' }
          ]
        }, Status.FAILED);
      this.assertionErrorMessages.push(`${message} expected:[${error.expected}] actual:[${error.actual}]`);
    }
  }

  assertAll() {
    let finalMessage = '';
    if (this.assertionErrorMessages.length === 0) {
      return;
    }
    for (let i = 0; i < this.assertionErrorMessages.length; i++) {
      if (i === 0) {
        finalMessage += `Following assertion(s) failed. Click here to see the details...\n\n`;
        finalMessage += `${(i + 1)}. ${this.assertionErrorMessages[i]}\n\n`;
      } else {
        finalMessage += `${(i + 1)}. ${this.assertionErrorMessages[i]}\n\n`;
      }
    }
    assert.isTrue(false, finalMessage);
  }

}

export default SoftAssert;
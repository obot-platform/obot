import { type Page, expect } from 'playwright/test';

export class BasePage {
	readonly page: Page;

	constructor(page: Page) {
		this.page = page;
	}

	async goto(path: string) {
		await this.page.goto(path);
		await this.hydrated();
	}

	async hydrated() {
		await expect(this.page.locator(':root')).toHaveAttribute('hydrated');
	}
}

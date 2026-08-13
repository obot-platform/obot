export {};

declare module 'vitest/browser' {
	interface Locator {
		locator(selector: string): Locator;
	}

	interface LocatorSelectors {
		getByCSS(selector: string): Locator;
	}
}

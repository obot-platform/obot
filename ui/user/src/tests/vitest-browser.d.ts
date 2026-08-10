import type { Locator } from 'vitest/browser';

declare module 'vitest/browser' {
	interface LocatorSelectors {
		getByCSS(selector: string): Locator;
		locator(selector: string): Locator;
	}
}

import type { Locator } from 'vitest/browser';

declare module 'vitest/browser' {
	interface LocatorSelectors {
		locator(selector: string): Locator;
	}
}

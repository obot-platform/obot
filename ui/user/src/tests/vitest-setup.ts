import '../app.css';
import { useShortTimeouts } from './helpers/shortTimeouts';
import { worker } from './mocks/worker';
import 'devicon/devicon.min.css';
import { beforeAll, beforeEach, afterEach, afterAll } from 'vitest';
import { locators } from 'vitest/browser';

locators.extend({
	getByCSS(selector) {
		return `css=${selector}`;
	}
});

let restoreTimeouts: (() => void) | undefined;

beforeAll(async () => {
	restoreTimeouts = useShortTimeouts();
	await worker.start({ onUnhandledRequest: 'error' });
});

beforeEach(() => {
	localStorage.clear();
	sessionStorage.clear();
});

afterEach(async () => {
	await worker.resetHandlers();
});

afterAll(async () => {
	restoreTimeouts?.();
	await worker.stop();
});

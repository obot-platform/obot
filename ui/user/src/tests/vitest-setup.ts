import '../app.css';
import { worker } from './mocks/worker';
import 'devicon/devicon.min.css';
import { beforeAll, beforeEach, afterEach, afterAll } from 'vitest';
import { locators } from 'vitest/browser';

locators.extend({
	getByCSS(selector) {
		return `css=${selector}`;
	}
});

beforeAll(async () => {
	await worker.start({ onUnhandledRequest: 'error' });
});

beforeEach(() => {
	localStorage.clear();
});

afterEach(async () => {
	await worker.resetHandlers();
});

afterAll(async () => {
	await worker.stop();
});

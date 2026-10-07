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
	await worker.start({
		quiet: true,
		onUnhandledRequest(request, print) {
			// Static files are served by the app. Let them through instead of failing the handler.
			if (/\.(?:svg|png|jpe?g|gif|webp|ico|woff2?)$/i.test(new URL(request.url).pathname)) return;
			print.error();
			throw new Error(`Unhandled ${request.method} ${new URL(request.url).pathname}`);
		}
	});
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

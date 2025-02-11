import "@testing-library/jest-dom";
import { mutate } from "swr";
import { server } from "test/server";

declare module "vitest" {
	interface Assertion {
		notToHaveUnhandledCalls(): void;
	}
}

// for ThemeProvider, mock window.matchMedia
Object.defineProperty(window, "matchMedia", {
	writable: true,
	value: vi.fn().mockImplementation((query) => ({
		matches: false,
		media: query,
		onchange: null,
		addListener: vi.fn(), // deprecated
		removeListener: vi.fn(), // deprecated
		addEventListener: vi.fn(),
		removeEventListener: vi.fn(),
		dispatchEvent: vi.fn(),
	})),
});

// mock ResizeObserver
global.ResizeObserver = class ResizeObserver {
	observe() {}
	unobserve() {}
	disconnect() {}
};

expect.extend({
	notToHaveUnhandledCalls(received) {
		const pass = received.mock.calls.length === 0;

		if (!pass) {
			const unhandledRequests = received.mock.calls
				.map(([req]: [Request]) => `${req.method} ${req.url}`)
				.join("\n  ");

			return {
				pass,
				message: () =>
					`[MSW] Error: intercepted a request without a matching request handler:\n\n  ${unhandledRequests}\n\n Make sure to add appropriate request handlers for these calls. \n Read more: https://mswjs.io/docs/getting-started/mocks`,
			};
		}
		return { pass, message: () => "No unhandled calls detected" };
	},
});

const onUnhandledRequest = vi.fn();

// Establish API mocking before all tests
beforeAll(() =>
	server.listen({
		onUnhandledRequest: onUnhandledRequest,
	})
);

beforeEach(() => {
	// Clear the SWR cache before each test
	mutate(() => true, undefined, { revalidate: true });
	onUnhandledRequest.mockClear();
});

// Reset any request handlers that we may add during the tests,
// so they don't affect other tests.
afterEach(() => {
	server.resetHandlers();
	expect(onUnhandledRequest).notToHaveUnhandledCalls();
});

// Clean up after the tests are finished.
afterAll(() => server.close());

import { Mock } from "vitest";
import { z } from "zod";

import {
	CreateFetcherReturn,
	createFetcher,
} from "~/lib/service/api/service-primitives";
import { revalidateObject } from "~/lib/service/revalidation";

vi.mock("~/lib/service/revalidation.ts", () => ({
	revalidateObject: vi.fn(),
}));
const mockRevalidateObject = revalidateObject as Mock;

describe(createFetcher, () => {
	const test = new TestFetcher();

	beforeEach(() => test.setup());

	it("should return fetcher values", () => {
		expect(test.fetcher).toEqual({
			handler: expect.any(Function),
			key: expect.any(Function),
			swr: expect.any(Function),
			revalidate: expect.any(Function),
		});
	});

	it("should trigger handler when handler is called", async () => {
		const params = { test: "test" };
		const config = { signal: new AbortController().signal };

		await test.fetcher.handler(params, config);
		expect(test.spy).toHaveBeenCalledWith(params, config);
	});

	it.each([
		[{ test: "test-value" }, { test: "test-value" }],
		[{ test: 1231 }, { test: undefined }],
		[{ test: null }, { test: undefined }],
		[{}, { test: undefined }],
		[undefined, { test: undefined }],
	])(
		"should trigger revalidation and skip invalid params when revalidate is called",
		(params, expected) => {
			// @ts-expect-error - we want to test the error case
			test.fetcher.revalidate(params);
			expect(mockRevalidateObject).toHaveBeenCalledWith({
				key: test.keyVal,
				params: expected,
			});
		}
	);

	describe(test.fetcher.swr, () => {
		it("should return a tuple with the key and the fetcher", () => {
			const params = { test: "test" };

			const [key, fetcher] = test.fetcher.swr(params);

			expect(key).toEqual({ key: test.keyVal, params });
			expect(fetcher).toEqual(expect.any(Function));
		});

		it("should return null for the key when disabled", () => {
			const [key] = test.fetcher.swr({ test: "test" }, { enabled: false });
			expect(key).toBeNull();
		});

		it.each([
			["empty", {}, null],
			["invalid-type", { test: 1234123 }, null], // number is not a valid value for `params.test`
		])(
			"should return null for the key when params are %s",
			(_, params, expected) => {
				// @ts-expect-error - we want to test the error case
				const [key] = test.fetcher.swr(params, { enabled: true });
				expect(key).toEqual(expected);
			}
		);

		it("should cancel duplicate requests", async () => {
			const params = { test: "test" };

			const abortControllerMock = mockAbortController();
			// setup spy this way to prevent the promise from resolving without using setTimeout
			const [spy, actions] = mockPromise();

			const test = new TestFetcher().setup({ spy });

			const [_, fetcher] = test.fetcher.swr(params);

			expect(spy).not.toHaveBeenCalled();
			expect(abortControllerMock.mock.abort).not.toHaveBeenCalled();

			// first trigger sets the abortController
			fetcher();

			expect(spy).toHaveBeenCalledTimes(1);
			expect(abortControllerMock.mock.abort).not.toHaveBeenCalled();

			// second trigger aborts
			fetcher();

			expect(spy).toHaveBeenCalledTimes(2);
			expect(abortControllerMock.mock.abort).toHaveBeenCalledTimes(1);

			// cleanup
			actions.resolve!(); // resolve the promise to allow garbage collection
			abortControllerMock.cleanup();
		});

		it("should not cancel duplicate requests when cancellable is false", async () => {
			const params = { test: "test" };

			const abortControllerMock = mockAbortController();
			// setup spy this way to prevent the promise from resolving without using setTimeout
			const [spy, actions] = mockPromise();

			const test = new TestFetcher().setup({ spy });

			const [_, fetcher] = test.fetcher.swr(params, { cancellable: false });

			expect(spy).not.toHaveBeenCalled();
			expect(abortControllerMock.mock.abort).not.toHaveBeenCalled();

			// first trigger sets the abortController
			fetcher();

			expect(spy).toHaveBeenCalledTimes(1);
			expect(abortControllerMock.mock.abort).not.toHaveBeenCalled();

			// second trigger aborts
			fetcher();

			expect(spy).toHaveBeenCalledTimes(2);
			expect(abortControllerMock.mock.abort).not.toHaveBeenCalled();

			// cleanup
			actions.resolve!(); // resolve the promise to allow garbage collection
			abortControllerMock.cleanup();
		});
	});
});

class TestFetcher {
	fetcher!: CreateFetcherReturn<{ test: string }, unknown>;
	spy!: Mock;
	keyVal!: string;

	constructor() {
		this.setup();
	}

	setup(config?: { spy?: Mock; keyVal?: string }) {
		const { spy = vi.fn(), keyVal = "test-key" } = config ?? {};

		this.spy = spy;
		this.keyVal = keyVal;

		this.fetcher = createFetcher(
			z.object({ test: z.string() }),
			this.spy,
			() => this.keyVal
		);

		return this;
	}
}

function mockAbortController() {
	const tempAbortController = AbortController;
	const mock = { abort: vi.fn(), signal: vi.fn() };

	// @ts-expect-error - the internal abort controller is hidden via a closure
	global.AbortController = vi.fn(() => mock);

	return {
		mock,
		cleanup: () => (global.AbortController = tempAbortController),
	};
}

function mockPromise() {
	const actions: { resolve?: () => void } = { resolve: undefined };

	const spy = vi.fn(() => {
		const mp = new Promise((res) => {
			actions.resolve = () => res(null);
		});

		return mp;
	});

	return [spy, actions] as const;
}

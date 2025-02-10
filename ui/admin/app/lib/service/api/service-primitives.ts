import { ZodRawShape, z } from "zod";

import { type KeyObj, revalidateObject } from "~/lib/service/revalidation";

export type FetcherConfig = {
	signal?: AbortSignal;
	cancellable?: boolean;
};

type FetcherSWR<TParams extends object, TResponse> = (
	params: NullishPartial<TParams>,
	config?: FetcherConfig & {
		enabled?: boolean;
	}
) => [Nullish<KeyObj<TParams>>, () => Promise<TResponse>];

type FetcherHandler<TParams extends object, TResponse> = (
	params: TParams,
	config?: FetcherConfig
) => Promise<TResponse>;

type FetcherRevalidate<TParams extends object> = (
	params?: NullishPartial<TParams>
) => void;

type FetcherKey = () => string;

export type CreateFetcherReturn<TParams extends object, TResponse> = {
	handler: FetcherHandler<TParams, TResponse>;
	key: FetcherKey;
	swr: FetcherSWR<TParams, TResponse>;
	revalidate: FetcherRevalidate<TParams>;
};

/**
 * Creates a fetcher for a given API function
 * @param input - The input schema
 * @param handler - The API function
 * @param key - The function that generates the UNIQUE key for the given params. This should include all dependencies of the method
 * @returns The fetcher
 */
export const createFetcher = <TParams extends object, TResponse>(
	input: z.ZodSchema<TParams>,
	handler: (params: TParams, config: FetcherConfig) => Promise<TResponse>,
	key: FetcherKey
): CreateFetcherReturn<TParams, TResponse> => {
	type KeyParams = NullishPartial<TParams>;

	/** Creates a closure to trigger abort controller on consecutive requests */
	let abortController: AbortController;

	// this is a hack to get the shape of the input schema
	// we need to do this because zod doesn't support getting the shape of a z.Schema
	const getShape = () => {
		if (!(input instanceof z.ZodObject)) {
			throw new Error("Input must be a ZodObject");
		}

		return (input as z.ZodObject<ZodRawShape>).shape;
	};

	// this schema will skip any missing REQUIRED parameters
	const skippedSchema = z.object(
		Object.fromEntries(
			Object.entries(getShape()).map(([key, schema]) => [
				key,
				// this means that if a parameter would cause an error, it will be skipped
				schema.optional().default(undefined).catch(undefined),
			])
		)
	);

	// this function will return null if the params are invalid
	// SWR will not call the handler if the key is null
	const buildKey = (params: KeyParams): Nullish<KeyObj<TParams>> => {
		const { data } = input.safeParse(params);
		return data ? { key: key(), params: data } : null;
	};

	const revalidate = (params: KeyParams = {}) => {
		const data = skippedSchema.parse(params);

		const keyObj = { key: key(), params: data };
		revalidateObject(keyObj);
	};

	const handleFetch = (params: TParams, config: FetcherConfig) => {
		const { cancellable = true } = config;

		if (cancellable) {
			abortController?.abort();
			abortController = new AbortController();
		}

		return handler(params, { signal: abortController?.signal, ...config });
	};

	return {
		handler: (params, config = {}) => handleFetch(params, config),
		key,
		/** Creates a SWR key and fetcher for the given params. This works for both `useSWR` and `prefetch` from SWR */
		swr: (params, config = {}) => {
			const { enabled = true, ...restConfig } = config;

			return [
				enabled ? buildKey(params) : null,
				// casting (params as TParams) is safe here because handleFetch will never be called when params are invalid
				() => handleFetch(params as TParams, restConfig),
			] as const;
		},
		revalidate,
	};
};

/**
 * Creates a mutator for a given API function
 * @param fn - The API function
 * @returns The mutator
 */
export const createMutator = <TInput extends object, TResponse>(
	fn: (params: TInput, config: FetcherConfig) => Promise<TResponse>
) => {
	/** Creates a closure to trigger abort controller on consecutive requests */
	let abortController: AbortController;

	return (params: TInput, config: FetcherConfig = {}) => {
		// cancellable is not defaulted to true for mutations
		const { cancellable } = config;

		if (cancellable) {
			abortController?.abort();
			abortController = new AbortController();
		}

		abortController = new AbortController();
		return fn(params, { signal: abortController.signal, ...config });
	};
};

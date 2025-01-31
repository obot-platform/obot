import { mutate } from "swr";
import { ZodObject, ZodRawShape, z } from "zod";

type FetcherConfig = {
	signal?: AbortSignal;
};

/**
 * Allows us to skip matching on specific key segments
 * @example
 * revalidateArray(["Agents", SkipKey])
 * // will revalidate the following keys:
 * ["Agents", "1234"]
 * ["Agents", "5678"]
 * // but not:
 * ["Agents", "1234", "Threads"]
 * ["Agents", "1234", "Threads", "5678"]
 *
 * // If exact is false:
 * revalidateArray(["Agents", SkipKey], false)
 * // will also revalidate the following keys:
 * ["Agents", "1234", "Threads"]
 * ["Agents", "1234", "Threads", "5678"]
 */
export const SkipKey = Symbol("SkipKey");

/**
 * Revalidates all keys that match the given key
 * @param key - The key to match. Use SkipKey to skip matching on specific segments
 * @param exact - Whether the key must match exactly
 */
export const revalidateArray = <TKey extends unknown[]>(
	key: TKey,
	exact = true
) =>
	mutate((cacheKey) => {
		if (!Array.isArray(cacheKey)) return false;

		return (
			key.every((k, i) => [cacheKey[i], SkipKey].includes(k)) &&
			(!exact || cacheKey.length === key.length)
		);
	});

/**
 * Creates a fetcher for a given API function
 * @param input - The input schema
 * @param handler - The API function
 * @param key - The function that generates the UNIQUE key for the given params. This should include all dependencies of the method
 * @returns The fetcher
 */
export const createFetcher = <
	TInput extends ZodObject<ZodRawShape>,
	TParams extends z.infer<TInput>,
	TKey extends unknown[],
	TResponse,
>(
	input: TInput,
	handler: (params: TParams, config?: FetcherConfig) => Promise<TResponse>,
	key: (params: TParams) => TKey
) => {
	type KeyParams = NullishPartial<TParams>;

	/** Creates a closure to trigger abort controller on consecutive requests */
	let abortController: AbortController;

	// this schema will skip any parameters that would cause an error
	const skippedSchema = z.object(
		Object.fromEntries(
			Object.entries(input.shape).map(([key, schema]) => [
				key,
				// this means that if a parameter would cause an error, it will be skipped
				schema.catch(SkipKey),
			])
		)
	);

	// this function will return null if the params are invalid
	// SWR will not call the handler if the key is null
	const buildKey = (params: KeyParams) => {
		const { data } = input.safeParse(params);
		return data ? key(data as TParams) : null;
	};

	const buildRevalidator = (params: KeyParams, exact?: boolean) => {
		const data = skippedSchema.parse(params);
		revalidateArray(key(data as TParams), exact);
	};

	const handleFetch = (params: TParams, config: FetcherConfig) => {
		abortController?.abort();

		abortController = new AbortController();
		return handler(params, { signal: abortController.signal, ...config });
	};

	return {
		handler,
		key,
		/** Creates a SWR key and fetcher for the given params. This works for both `useSWR` and `prefetch` from SWR */
		swr: (params: KeyParams, config: FetcherConfig = {}) => {
			return [
				buildKey(params),
				// casting (params as TParams) is safe here because handleFetch will never be called when params are invalid
				() => handleFetch(params as TParams, config),
			] as const;
		},
		revalidate: (params: KeyParams, exact?: boolean) =>
			buildRevalidator(params, exact),
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
		abortController?.abort();

		abortController = new AbortController();
		return fn(params, { signal: abortController.signal, ...config });
	};
};

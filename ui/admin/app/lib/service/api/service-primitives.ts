import { mutate } from "swr";
import { ZodObject, ZodRawShape, z } from "zod";

type FetcherConfig = {
	signal?: AbortSignal;
};

export const RevalidateSkip = Symbol("RevalidateSkip");

export const revalidateArray = <TKey extends unknown[]>(
	key: TKey,
	exact = true
) =>
	mutate((cacheKey) => {
		if (!Array.isArray(cacheKey)) return false;

		return (
			key.every((k, i) => [cacheKey[i], RevalidateSkip].includes(k)) &&
			(!exact || cacheKey.length === key.length)
		);
	});

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

	const buildKey = (params: KeyParams) => {
		const { data } = input.safeParse(params);
		return data ? key(data as TParams) : null;
	};

	const skippedSchema = z.object(
		Object.fromEntries(
			Object.entries(input.shape).map(([key, schema]) => [
				key,
				schema.catch(RevalidateSkip),
			])
		)
	);

	const buildRevalidator = (params: KeyParams, exact?: boolean) => {
		const data = skippedSchema.parse(params);
		revalidateArray(key(data as TParams), exact);
	};

	// create a closure to trigger abort controller on consecutive requests
	let abortController: AbortController;

	const handleFetch = (params: TParams, config: FetcherConfig) => {
		abortController?.abort();

		abortController = new AbortController();
		return handler(params, { signal: abortController.signal, ...config });
	};

	return {
		handler,
		key,
		swr: (params: KeyParams, config: FetcherConfig = {}) => {
			return [
				buildKey(params),
				() => handleFetch(params as TParams, config),
			] as const;
		},
		revalidate: (params: KeyParams, exact?: boolean) =>
			buildRevalidator(params, exact),
	};
};

export const createMutator = <TInput extends object, TResponse>(
	fn: (params: TInput, config: FetcherConfig) => Promise<TResponse>
) => {
	let abortController: AbortController;

	return (params: TInput, config: FetcherConfig = {}) => {
		abortController?.abort();

		abortController = new AbortController();
		return fn(params, { signal: abortController.signal, ...config });
	};
};

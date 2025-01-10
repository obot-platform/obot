const forceError = (
	error: unknown,
	fallbackMessage = "Something went wrong"
) => {
	if (error instanceof Error) return error;

	if (typeof error === "string") return new Error(error);

	return new Error(fallbackMessage, { cause: error });
};

export type PromiseResult<TData> =
	| readonly [null, TData]
	| readonly [Error, null];

export async function handlePromise<TData>(
	promise: Promise<TData>,
	config?: { fallbackMessage?: string }
): Promise<PromiseResult<TData>> {
	try {
		return [null, await promise] as const;
	} catch (error) {
		return [forceError(error, config?.fallbackMessage), null] as const;
	}
}

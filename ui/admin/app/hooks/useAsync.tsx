import { useCallback, useState } from "react";

import { BoundaryError } from "~/lib/service/api/apiErrors";
import { PromiseResult, handlePromise } from "~/lib/utils/handlePromise";

type Config<TData, TParams extends unknown[]> = {
	onSuccess?: (data: TData, params: TParams) => void;
	onError?: (error: unknown, params: TParams) => void;
	onSettled?: ({ params }: { params: TParams }) => void;
	shouldThrow?: (error: unknown) => boolean;
};

type AsyncState<TData, TParams extends unknown[]> = {
	data: TData | null;
	error: unknown;
	isLoading: boolean;
	lastCallParams: TParams | null;
	execute: (...params: TParams) => void;
	executeAsync: (...params: TParams) => Promise<PromiseResult<TData>>;
	clear: () => void;
};

const defaultShouldThrow = (error: unknown) => error instanceof BoundaryError;

export function useAsync<TData, TParams extends unknown[]>(
	callback: (...params: TParams) => Promise<TData>,
	config?: Config<TData, TParams>
): AsyncState<TData, TParams> {
	const {
		onSuccess,
		onError,
		onSettled,
		shouldThrow = defaultShouldThrow,
	} = config || {};

	const [data, setData] = useState<TData | null>(null);
	const [error, setError] = useState<unknown>(null);
	const [isLoading, setIsLoading] = useState(false);
	const [lastCallParams, setLastCallParams] = useState<TParams | null>(null);

	if (error && shouldThrow(error)) throw error;

	const executeAsync = useCallback(
		async (...params: TParams) => {
			setIsLoading(true);
			setData(null);
			setLastCallParams(params);
			const promise = callback(...params);

			promise
				.then((data) => {
					setData(data);
					onSuccess?.(data, params);
				})
				.catch((error) => {
					// If the response data is a JSON string with an error field
					// unpack that error field's value into the error message
					if (error.response && typeof error.response.data === "string") {
						const errorMessageMatch =
							error.response.data.match(/{"error":\s+"(.*?)"}/);
						if (errorMessageMatch) {
							const errorMessage = JSON.parse(errorMessageMatch[0]).error;
							console.log("Error: ", errorMessage);
							error.message = errorMessage;
						}
					}
					setError(error);
					onError?.(error, params);
				})
				.finally(() => {
					setIsLoading(false);
					onSettled?.({ params });
				});

			return await handlePromise(promise);
		},
		[callback, onSuccess, onError, onSettled]
	);

	const execute = useCallback(
		(...params: TParams) => {
			executeAsync(...params);
		},
		[executeAsync]
	);

	const clear = useCallback(() => {
		setData(null);
		setError(null);
	}, []);

	return {
		data,
		error,
		isLoading,
		lastCallParams,
		execute,
		executeAsync,
		clear,
	};
}

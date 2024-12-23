import { useCallback, useState } from "react";

import { BoundaryError } from "~/lib/service/api/apiErrors";
import { handlePromise } from "~/lib/service/async";

type Config<TData, TParams extends unknown[]> = {
    onSuccess?: (data: TData, params: TParams) => void;
    onError?: (error: unknown, params: TParams) => void;
    onSettled?: ({ params }: { params: TParams }) => void;
    shouldThrow?: (error: unknown) => boolean;
};

const defaultShouldThrow = (error: unknown) => error instanceof BoundaryError;

export function useAsync<TData, TParams extends unknown[]>(
    callback: (...params: TParams) => Promise<TData>,
    config?: Config<TData, TParams>
) {
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
                    if (
                        error.response &&
                        typeof error.response.data === "string"
                    ) {
                        const errorMessageMatch =
                            error.response.data.match(/{"error":\s+"(.*?)"}/);
                        if (errorMessageMatch) {
                            const errorMessage = JSON.parse(
                                errorMessageMatch[0]
                            ).error;
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

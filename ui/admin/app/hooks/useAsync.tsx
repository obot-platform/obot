import { useCallback, useState } from "react";

import { noop } from "~/lib/utils";

type Config<TData, TParams extends unknown[]> = {
    onSuccess?: (data: TData, params: TParams) => void;
    onError?: (error: unknown, params: TParams) => void;
    onSettled?: ({ params }: { params: TParams }) => void;
};

export function useAsync<TData, TParams extends unknown[]>(
    callback: (...params: TParams) => Promise<TData>,
    config?: Config<TData, TParams>
) {
    const { onSuccess, onError, onSettled } = config || {};

    const [data, setData] = useState<TData | null>(null);
    const [error, setError] = useState<unknown>(null);
    const [isLoading, setIsLoading] = useState(false);
    const [lastCallParams, setLastCallParams] = useState<TParams | null>(null);

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
                    setError(error);
                    onError?.(error, params);
                })
                .finally(() => {
                    setIsLoading(false);
                    onSettled?.({ params });
                });

            return promise;
        },
        [callback, onSuccess, onError, onSettled]
    );

    const execute = useCallback(
        (...params: TParams) => {
            executeAsync(...params).catch(noop);
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

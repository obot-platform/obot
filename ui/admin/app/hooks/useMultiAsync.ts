import { useCallback, useState } from "react";

import { handlePromise } from "~/lib/service/async";
import { noop } from "~/lib/utils";

type MultiConfig<TData, TParams extends unknown[]> = {
    onSuccess?: (data: TData[], params: TParams[]) => void;
    onError?: (error: unknown[], params: TParams[]) => void;
    onSettled?: ({ params }: { params: TParams[] }) => void;
};

export type AsyncState<TData, TParams extends unknown[]> = {
    data: TData | null;
    error: unknown | null;
    isLoading: boolean;
    isSuccessful: boolean;
    params: TParams;
};

export function useMultiAsync<TData, TParams extends unknown[]>(
    callback: (index: number, ...params: TParams) => Promise<TData>,
    config?: MultiConfig<TData, TParams>
) {
    const { onSuccess, onError, onSettled } = config || {};

    const [states, setStates] = useState<AsyncState<TData, TParams>[]>([]);

    const executeAsync = useCallback(
        async (paramsList: TParams[]) => {
            setStates(
                paramsList.map((params) => ({
                    data: null,
                    error: null,
                    isLoading: true,
                    isSuccessful: false,
                    params,
                }))
            );

            const promises = paramsList.map((params, index) => {
                const prom = callback(index, ...params);

                prom.then((result) => {
                    setStates((prevStates) => {
                        const newStates = [...prevStates];
                        newStates[index] = {
                            ...newStates[index],
                            data: result,
                            isLoading: false,
                            isSuccessful: true,
                        };
                        return newStates;
                    });
                    return result;
                }).catch((error) => {
                    setStates((prevStates) => {
                        const newStates = [...prevStates];
                        newStates[index] = {
                            ...newStates[index],
                            error,
                            isLoading: false,
                            isSuccessful: false,
                        };
                        return newStates;
                    });
                    console.error(error);
                });

                return prom;
            });

            try {
                const results = await Promise.all(promises);
                onSuccess?.(results, paramsList);
            } catch (err) {
                const errorArray = Array.isArray(err) ? err : [err];
                onError?.(errorArray, paramsList);
            } finally {
                onSettled?.({ params: paramsList });
            }

            return await Promise.all(promises.map((p) => handlePromise(p)));
        },
        [callback, onSuccess, onError, onSettled]
    );

    const execute = useCallback(
        (params: TParams[]) => {
            executeAsync(params).catch(noop);
        },
        [executeAsync]
    );

    const clear = useCallback(() => {
        setStates([]);
    }, []);

    return { states, execute, executeAsync, clear };
}

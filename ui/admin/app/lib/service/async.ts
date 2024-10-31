export const handlePromise = async <TData, TError = object>(
    promise: Promise<TData>
): Promise<{ data: TData; error: null } | { data: null; error: TError }> => {
    try {
        return { data: await promise, error: null };
    } catch (error) {
        return { data: null, error: error as TError };
    }
};

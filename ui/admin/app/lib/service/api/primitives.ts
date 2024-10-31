// TODO: Add default configurations with auth tokens, etc. When ready
import axios, { AxiosRequestConfig, AxiosResponse, isAxiosError } from "axios";

import { ConflictError } from "./apiErrors";

export const ResponseHeaders = {
    ThreadId: "x-otto-thread-id",
} as const;

const internalFetch = axios.request;

interface ExtendedAxiosRequestConfig<D = unknown>
    extends AxiosRequestConfig<D> {
    errorMessage?: string;
}

export async function request<T, R = AxiosResponse<T>, D = unknown>({
    errorMessage = "Request failed",
    ...config
}: ExtendedAxiosRequestConfig<D>): Promise<R> {
    try {
        return await internalFetch<T, R, D>({
            adapter: "fetch",
            ...config,
        });
    } catch (error) {
        console.error(errorMessage);

        if (isAxiosError(error) && error.response?.status === 409) {
            throw new ConflictError(error.response.data);
        }

        throw error;
    }
}

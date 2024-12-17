// TODO: Add default configurations with auth tokens, etc. When ready
import axios, { AxiosRequestConfig, AxiosResponse, isAxiosError } from "axios";

import { AuthDisabledUsername } from "~/lib/model/auth";
import { User } from "~/lib/model/users";
import { ApiRoutes } from "~/lib/routers/apiRoutes";
import {
    BadRequestError,
    ConflictError,
    ForbiddenError,
    NotFoundError,
    UnauthorizedError,
} from "~/lib/service/api/apiErrors";

export const ResponseHeaders = {
    ThreadId: "x-otto-thread-id",
} as const;

const internalFetch = axios.request;

interface ExtendedAxiosRequestConfig<D = unknown>
    extends AxiosRequestConfig<D> {
    errorMessage?: string;
    disableTokenRefresh?: boolean;
}

export async function request<T, R = AxiosResponse<T>, D = unknown>({
    errorMessage: _,
    disableTokenRefresh,
    ...config
}: ExtendedAxiosRequestConfig<D>): Promise<R> {
    try {
        return await internalFetch<T, R, D>({
            adapter: "fetch",
            ...config,
        });
    } catch (error) {
        if (isAxiosError(error) && error.response?.status === 400) {
            throw new BadRequestError(error.response.data);
        }

        if (isAxiosError(error) && error.response?.status === 401) {
            throw new UnauthorizedError(error.response.data);
        }

        if (isAxiosError(error) && error.response?.status === 403) {
            // Tokens are automatically refreshed on GET requests
            if (disableTokenRefresh) {
                throw new ForbiddenError(error.response.data);
            }

            console.info("Forbidden request, attempting to refresh token");
            const { data } = await internalFetch<User>({
                url: ApiRoutes.me().url,
            });

            // if token is refreshed successfully, retry the request
            if (!data?.username || data.username === AuthDisabledUsername)
                throw new ForbiddenError(error.response.data);

            console.info("Token refreshed");
            return request<T, R, D>({
                ...config,
                disableTokenRefresh: true,
            });
        }

        if (isAxiosError(error) && error.response?.status === 404) {
            throw new NotFoundError(error.response.data);
        }

        if (isAxiosError(error) && error.response?.status === 409) {
            throw new ConflictError(error.response.data);
        }

        throw error;
    }
}

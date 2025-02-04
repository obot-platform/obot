// TODO: Add default configurations with auth tokens, etc. When ready
import axios, {
	AxiosRequestConfig,
	AxiosResponse,
	CanceledError,
	isAxiosError,
} from "axios";
import { toast } from "sonner";

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
import { handlePromise } from "~/lib/utils/handlePromise";

export const ResponseHeaders = {
	ThreadId: "x-obot-thread-id",
} as const;

export const RequestHeaders = {
	UserTimezone: "x-obot-user-timezone",
} as const;

const internalFetch = axios.request;

interface ExtendedAxiosRequestConfig<D = unknown>
	extends AxiosRequestConfig<D> {
	errorMessage?: string;
	disableTokenRefresh?: boolean;
	debugThrow?: Error;
	toastError?: boolean;
}

export async function request<T, R = AxiosResponse<T>, D = unknown>({
	errorMessage = "Something went wrong",
	disableTokenRefresh,
	debugThrow,
	toastError = true,
	...config
}: ExtendedAxiosRequestConfig<D>): Promise<R> {
	// Get the browser's default timezone
	const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone;
	// Merge the existing headers with the new Timezone header
	const headers = {
		...config.headers,
		[RequestHeaders.UserTimezone]: timezone,
	};

	const [responseError, response] = await handlePromise(
		internalFetch<T, R, D>({
			adapter: "fetch",
			...config,
			headers,
		})
	);

	const error = responseError || debugThrow;
	if (error) {
		const convertedError = convertError(error);

		if (convertedError instanceof ForbiddenError && !disableTokenRefresh) {
			console.info("Forbidden request, attempting to refresh token");
			const { data } = await internalFetch<User>({
				url: ApiRoutes.me().url,
				headers: {
					[RequestHeaders.UserTimezone]: timezone,
				},
			});

			// if token is refreshed successfully, retry the request
			if (!data?.username || data.username === AuthDisabledUsername)
				throw convertedError;

			console.info("Token refreshed");
			return request<T, R, D>({
				...config,
				disableTokenRefresh: true,
			});
		}

		if (toastError && !(convertedError instanceof CanceledError))
			toast.error(errorMessage);

		throw convertedError;
	}

	return response;
}

function convertError(error: Error) {
	if (isAxiosError(error) && error.response?.status === 400) {
		return new BadRequestError(error.response.data);
	}

	if (isAxiosError(error) && error.response?.status === 401) {
		return new UnauthorizedError(error.response.data);
	}

	if (isAxiosError(error) && error.response?.status === 403) {
		return new ForbiddenError(error.response.data);
	}

	if (isAxiosError(error) && error.response?.status === 404) {
		return new NotFoundError(error.response.data);
	}

	if (isAxiosError(error) && error.response?.status === 409) {
		return new ConflictError(error.response.data);
	}

	if (isAxiosError(error) && error.code === "ERR_CANCELED") {
		return new CanceledError(error.name);
	}

	return error;
}

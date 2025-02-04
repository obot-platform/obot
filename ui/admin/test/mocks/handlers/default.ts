import { HttpResponse, http } from "msw";
import { mockedBootstrappedUser, mockedUser } from "test/mocks/models/users";
import { mockedVersion } from "test/mocks/models/version";

import { User } from "~/lib/model/users";
import { Version } from "~/lib/model/version";
import { ApiRoutes } from "~/lib/routers/apiRoutes";

export const defaultMockedHandlers = [
	http.get(ApiRoutes.bootstrap.status().path, () => {
		return HttpResponse.json<{ data: { enabled: boolean } }>({
			data: { enabled: false },
		});
	}),
	http.get(ApiRoutes.version().path, () => {
		return HttpResponse.json<Version>(mockedVersion);
	}),
	http.get(ApiRoutes.me().path, () => {
		return HttpResponse.json<{ data: User }>({
			data: mockedUser,
		});
	}),
];

export const defaultBootstrappedMockHandlers = [
	http.get(ApiRoutes.bootstrap.status().path, () => {
		return HttpResponse.json<{ data: { enabled: boolean } }>({
			data: { enabled: true },
		});
	}),
	http.get(ApiRoutes.version().path, () => {
		return HttpResponse.json<Version>(mockedVersion);
	}),
	http.get(ApiRoutes.me().path, () => {
		return HttpResponse.json<{ data: User }>({
			data: mockedBootstrappedUser,
		});
	}),
];

import { HttpResponse, http } from "msw";

import { mockedUser } from "~/lib/model/__mocks__/users";
import { mockedVersion } from "~/lib/model/__mocks__/version";
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
		return HttpResponse.json<{ data: Version }>({
			data: mockedVersion,
		});
	}),
	http.get(ApiRoutes.me().path, () => {
		return HttpResponse.json<{ data: User }>({
			data: mockedUser,
		});
	}),
];

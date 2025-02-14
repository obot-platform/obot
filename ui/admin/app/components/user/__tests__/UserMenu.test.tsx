import { HttpResponse, http } from "msw";
import { cleanup, render, screen } from "test";
import { mockedUser } from "test/mocks/models/users";
import { overrideServer } from "test/server";

import { CommonAuthProviderIds } from "~/lib/model/auth";
import { User } from "~/lib/model/users";
import { ApiRoutes } from "~/lib/routers/apiRoutes";

import { UserMenu } from "~/components/user/UserMenu";

describe(UserMenu, () => {
	afterEach(() => {
		cleanup();
		vi.clearAllMocks();
	});

	it("logging in with github oauth displays username", async () => {
		overrideServer([
			http.get(ApiRoutes.me().path, () => {
				return HttpResponse.json<User>({
					...mockedUser,
					currentAuthProvider: CommonAuthProviderIds.GITHUB,
				});
			}),
		]);

		render(<UserMenu />);
		const element = await screen.findByText(mockedUser.username);
		expect(element).toBeInTheDocument();
	});

	it("logging in with other oauth displays email", async () => {
		overrideServer([
			http.get(ApiRoutes.me().url, () => {
				return HttpResponse.json<User>({
					...mockedUser,
					currentAuthProvider: CommonAuthProviderIds.GOOGLE,
				});
			}),
		]);

		render(<UserMenu />);
		const element = await screen.findByText(mockedUser.email);
		expect(element).toBeInTheDocument();
	});
});

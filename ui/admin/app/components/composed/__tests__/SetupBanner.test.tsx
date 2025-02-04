import {
	HttpResponse,
	http,
	overrideServer,
	render,
	screen,
	waitFor,
} from "test";
import { defaultBootstrappedMockHandlers } from "test/mocks/handlers/default";
import { mockedAuthProvider } from "test/mocks/models/authProvider";
import { mockedModelProvider } from "test/mocks/models/modelProvider";

import { EntityList } from "~/lib/model/primitives";
import { AuthProvider, ModelProvider } from "~/lib/model/providers";
import { ApiRoutes } from "~/lib/routers/apiRoutes";

import { SetupBanner } from "~/components/composed/SetupBanner";

describe(SetupBanner, () => {
	const setupServer = (
		modelProviderConfigured: boolean,
		authProviderConfigured: boolean
	) => {
		overrideServer([
			...defaultBootstrappedMockHandlers,
			http.get(ApiRoutes.modelProviders.getModelProviders().url, () => {
				return HttpResponse.json<EntityList<ModelProvider>>({
					items: [
						{
							...mockedModelProvider,
							configured: modelProviderConfigured,
						},
					],
				});
			}),
			http.get(ApiRoutes.authProviders.getAuthProviders().url, () => {
				return HttpResponse.json<EntityList<AuthProvider>>({
					items: [
						{
							...mockedAuthProvider,
							configured: authProviderConfigured,
						},
					],
				});
			}),
		]);
	};

	it.each([
		["Both Options", false, false],
		["Only Model Provider Option", false, true],
		["OnlyAuth Provider Option", true, false],
		["Does Not Render Banner", true, true],
	])(
		"Renders: %s",
		async (_, modelProviderConfigured, authProviderConfigured) => {
			setupServer(modelProviderConfigured, authProviderConfigured);
			render(<SetupBanner />);

			await waitFor(() => {
				if (modelProviderConfigured) {
					expect(
						screen.queryByText("Configure Model Provider")
					).not.toBeInTheDocument();
				} else {
					expect(
						screen.getByText("Configure Model Provider")
					).toBeInTheDocument();
				}
				if (authProviderConfigured) {
					expect(
						screen.queryByText("Configure Auth Provider")
					).not.toBeInTheDocument();
				} else {
					expect(
						screen.getByText("Configure Auth Provider")
					).toBeInTheDocument();
				}
			});
		}
	);
});

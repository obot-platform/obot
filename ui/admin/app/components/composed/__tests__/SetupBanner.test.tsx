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

	it("Renders both steps when neither are configured", async () => {
		setupServer(false, false);
		render(<SetupBanner />);

		await waitFor(() => {
			expect(screen.getByText("Configure Model Provider")).toBeInTheDocument();
			expect(screen.getByText("Configure Auth Provider")).toBeInTheDocument();
		});
	});

	it("Renders model provider step when auth provider is configured", async () => {
		setupServer(false, true);
		render(<SetupBanner />);

		await waitFor(() => {
			expect(screen.getByText("Configure Model Provider")).toBeInTheDocument();
			expect(
				screen.queryByText("Configure Auth Provider")
			).not.toBeInTheDocument();
		});
	});

	it("Renders auth provider step when model provider is configured", async () => {
		setupServer(true, false);
		render(<SetupBanner />);

		await waitFor(() => {
			expect(screen.getByText("Configure Auth Provider")).toBeInTheDocument();
			expect(
				screen.queryByText("Configure Model Provider")
			).not.toBeInTheDocument();
		});
	});

	it("Does not render banner when both are configured", async () => {
		setupServer(true, true);
		render(<SetupBanner />);
		await waitFor(() => {
			expect(
				screen.queryByText("Configure Model Provider")
			).not.toBeInTheDocument();
			expect(
				screen.queryByText("Configure Auth Provider")
			).not.toBeInTheDocument();
		});
	});
});

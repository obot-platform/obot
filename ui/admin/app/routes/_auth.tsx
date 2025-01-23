import { AxiosError } from "axios";
import { Outlet, isRouteErrorResponse, useRouteError } from "react-router";
import { preload } from "swr";

import { ForbiddenError, UnauthorizedError } from "~/lib/service/api/apiErrors";
import { ModelProviderApiService } from "~/lib/service/api/modelProviderApiService";
import { UserService } from "~/lib/service/api/userService";

import { useAuth } from "~/components/auth/AuthContext";
import { SetupBanner } from "~/components/composed/SetupBanner";
import { Error, RouteError, Unauthorized } from "~/components/errors";
import { HeaderNav } from "~/components/header/HeaderNav";
import { Sidebar } from "~/components/sidebar";
import { SignIn } from "~/components/signin/SignIn";

export async function clientLoader() {
	const promises = await Promise.all([
		preload(UserService.getMe.key(), () => UserService.getMe()),
		preload(
			ModelProviderApiService.getModelProviders.key(),
			ModelProviderApiService.getModelProviders
		),
	]);
	const me = promises[0];

	return { me };
}

export default function AuthLayout() {
	return (
		<div className="flex h-screen w-screen overflow-hidden bg-background">
			<Sidebar />
			<div className="flex flex-grow flex-col overflow-hidden">
				<HeaderNav />
				<SetupBanner />
				<main className="flex-grow overflow-auto">
					<Outlet />
				</main>
			</div>
		</div>
	);
}

export function ErrorBoundary() {
	const error = useRouteError();
	const { isSignedIn } = useAuth();

	switch (true) {
		case error instanceof UnauthorizedError:
		case error instanceof ForbiddenError:
		case error instanceof AxiosError &&
			[401, 403].includes(error.response?.status ?? 0):
			if (isSignedIn) return <Unauthorized />;
			else return <SignIn />;
		case isRouteErrorResponse(error):
			return <RouteError error={error} />;
		default:
			return <Error error={error as Error} />;
	}
}

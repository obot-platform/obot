import { AxiosError } from "axios";
import { Outlet, isRouteErrorResponse, useRouteError } from "react-router";
import { preload } from "swr";

import { Role, User } from "~/lib/model/users";
import { ForbiddenError, UnauthorizedError } from "~/lib/service/api/apiErrors";
import { AuthProviderApiService } from "~/lib/service/api/authProviderApiService";
import { ModelProviderApiService } from "~/lib/service/api/modelProviderApiService";
import { UserService } from "~/lib/service/api/userService";

import { useAuth } from "~/components/auth/AuthContext";
import { SetupBanner } from "~/components/composed/SetupBanner";
import { Error, RouteError, Unauthorized } from "~/components/errors";
import { HeaderNav } from "~/components/header/HeaderNav";
import { Sidebar } from "~/components/sidebar";
import { SignIn } from "~/components/signin/SignIn";

export async function clientLoader() {
	let me: User | undefined;
	try {
		me = await preload(UserService.getMe.key(), UserService.getMe);
		if (me.role === Role.Admin) {
			await preload(
				ModelProviderApiService.getModelProviders.key(),
				ModelProviderApiService.getModelProviders
			);
		}
	} catch (error) {
		await preload(
			AuthProviderApiService.getAuthProviders.key(),
			AuthProviderApiService.getAuthProviders
		);
		throw error;
	}
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

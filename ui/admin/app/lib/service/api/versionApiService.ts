import { Version } from "~/lib/model/version";
import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { NotFoundError } from "~/lib/service/api/apiErrors";
import { request } from "~/lib/service/api/primitives";

async function getVersion() {
	const res = await request<Version>({
		url: ApiRoutes.version().url,
		errorMessage: "Failed to fetch app version.",
		toastError: false,
	});

	return res.data;
}
getVersion.key = () => ({ url: ApiRoutes.version().path }) as const;

async function requireAuthEnabled() {
	const { authEnabled } = await getVersion();
	if (!authEnabled) {
		throw new NotFoundError("Authentication is not enabled.");
	}
}

export const VersionApiService = {
	getVersion,
	requireAuthEnabled,
};

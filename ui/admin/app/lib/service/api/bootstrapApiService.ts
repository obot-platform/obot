import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { request } from "~/lib/service/api/primitives";

async function bootstrapLogin(token: string) {
	await request({
		method: "POST",
		url: ApiRoutes.bootstrap.login().url,
		headers: {
			Authorization: `Bearer ${token}`,
		},
	});
}

export const BootstrapApiService = {
	bootstrapLogin,
};

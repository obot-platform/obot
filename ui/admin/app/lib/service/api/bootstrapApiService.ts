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

async function bootstrapLogout() {
	await request({
		method: "POST",
		url: ApiRoutes.bootstrap.logout().url,
	});
}

async function bootstrapStatus() {
	const { data } = await request<{ enabled: boolean }>({
		method: "GET",
		url: ApiRoutes.bootstrap.status().url,
	});

	return data;
}
bootstrapStatus.key = () => ({ url: ApiRoutes.bootstrap.status().path });

export const BootstrapApiService = {
	bootstrapLogin,
	bootstrapLogout,
	bootstrapStatus,
};

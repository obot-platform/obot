import { z } from "zod";

import { EntityList } from "~/lib/model/primitives";
import { User } from "~/lib/model/users";
import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { request } from "~/lib/service/api/primitives";
import { createFetcher } from "~/lib/service/api/service-primitives";

const handleGetUsers = createFetcher(
	z.object({
		filters: z.object({ userId: z.string().optional() }).optional(),
	}),
	async ({ filters = {} }, { signal }) => {
		const { url } = ApiRoutes.users.base();
		const { data } = await request<EntityList<User>>({ url, signal });

		const { userId } = filters;

		if (userId) data.items = data.items?.filter((u) => u.id === userId);

		return data.items ?? [];
	},
	() => ApiRoutes.users.base().path
);

async function getMe() {
	const res = await request<User>({
		url: ApiRoutes.me().url,
		errorMessage: "Failed to fetch agents",
	});

	return res.data;
}
getMe.key = () => ({ url: ApiRoutes.me().path }) as const;

async function updateUser(username: string, user: Partial<User>) {
	const { data } = await request<User>({
		url: ApiRoutes.users.updateUser(username).url,
		method: "PATCH",
		data: user,
		errorMessage: "Failed to update user",
	});

	return data;
}

const handleGetUser = createFetcher(
	z.object({ username: z.string() }),
	async ({ username }, { signal }) => {
		const { url } = ApiRoutes.users.getOne(username);
		const { data } = await request<User>({ url, signal });
		return data;
	},
	() => ApiRoutes.users.getOne(":username").path
);

export const UserService = {
	getMe,
	getUsers: handleGetUsers,
	updateUser,
	getUser: handleGetUser,
};

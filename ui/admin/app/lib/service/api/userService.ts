import { User } from "~/lib/model/users";
import { ApiRoutes, revalidateWhere } from "~/lib/routers/apiRoutes";
import { request } from "~/lib/service/api/primitives";

async function getUsers() {
	const res = await request<{ items: User[] }>({
		url: ApiRoutes.users.base().url,
		errorMessage: "Failed to fetch users",
	});

	return res.data.items ?? ([] as User[]);
}
getUsers.key = () => ({ url: ApiRoutes.users.base().path }) as const;
getUsers.revalidate = () =>
	revalidateWhere((url) => url.includes(ApiRoutes.users.base().path));

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

export const UserService = {
	getMe,
	getUsers,
	updateUser,
};

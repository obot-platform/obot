import { AxiosError } from "axios";

import { UserService } from "~/lib/service/api/userService";

export const signedIn = async () => {
	try {
		await UserService.getMe();
	} catch (error) {
		if (error instanceof AxiosError && error.response?.status === 403) {
			return false;
		}
	}
	return true;
};

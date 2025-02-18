import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { request } from "~/lib/service/api/primitives";

async function invokeAsync({
	slug,
	prompt,
	thread,
}: {
	slug: string;
	prompt?: Nullish<string>;
	thread?: Nullish<string>;
}) {
	const { data } = await request<{ threadID: string; runID: string }>({
		url: ApiRoutes.invoke(slug, thread, { async: true }).url,
		method: "POST",
		data: prompt,
		errorMessage: "Failed to invoke agent",
	});

	return data;
}

export const InvokeService = {
	invokeAgent: invokeAsync,
};

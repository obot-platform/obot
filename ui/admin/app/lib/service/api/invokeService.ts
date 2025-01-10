import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { ResponseHeaders, request } from "~/lib/service/api/primitives";

async function invokeWithStream({
	slug,
	prompt,
	thread,
}: {
	slug: string;
	prompt?: string | null;
	thread?: string | null;
}) {
	const response = await request<ReadableStream>({
		url: ApiRoutes.invoke(slug, thread).url,
		method: "POST",
		headers: { Accept: "text/event-stream" },
		responseType: "stream",
		data: prompt,
		errorMessage: "Failed to invoke agent",
	});

	const reader = response.data
		?.pipeThrough(new TextDecoderStream())
		.getReader();

	const threadId = response.headers[
		ResponseHeaders.ThreadId
	] as Nullish<string>;

	return { reader, threadId };
}

const invokeAgentWithStream = invokeWithStream;
const invokeWorkflowWithStream = invokeWithStream;

export const InvokeService = {
	invokeAgentWithStream,
	invokeWorkflowWithStream,
};

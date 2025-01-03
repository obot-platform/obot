import { AssistantNamespace } from "~/lib/model/assistants";
import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { ResponseHeaders, request } from "~/lib/service/api/primitives";

async function authenticateTools(
    namespace: AssistantNamespace,
    entityId: string,
    tools: string[]
) {
    const response = await request<ReadableStream>({
        url: ApiRoutes.toolAuthentication.authenticate(namespace, entityId).url,
        method: "POST",
        headers: { Accept: "text/event-stream" },
        responseType: "stream",
        data: tools,
        errorMessage: "Failed to authenticate tools",
    });

    const reader = response.data
        ?.pipeThrough(new TextDecoderStream())
        .getReader();

    const threadId = response.headers[
        ResponseHeaders.ThreadId
    ] as Nullish<string>;

    return { reader, threadId };
}

async function deauthenticateTools(
    namespace: AssistantNamespace,
    entityId: string,
    tools: string[]
) {
    await request({
        url: ApiRoutes.toolAuthentication.deauthenticate(namespace, entityId)
            .url,
        method: "POST",
        data: tools,
        errorMessage: "Failed to deauthenticate tools",
    });
}

export const ToolAuthApiService = {
    authenticateTools,
    deauthenticateTools,
};

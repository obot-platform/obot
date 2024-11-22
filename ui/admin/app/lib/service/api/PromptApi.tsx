import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { request } from "~/lib/service/api/primitives";

async function promptResponse(prompt: {
    id?: string;
    response?: Record<string, string>;
}) {
    await request({
        method: "POST",
        url: ApiRoutes.prompt.promptResponse().url,
        data: prompt,
    });
}
export const PromptApiService = { promptResponse };

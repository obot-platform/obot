import queryString from "query-string";
import { mutate } from "swr";

import { ToolReferenceType } from "~/lib/model/toolReferences";
import { ApiUrl } from "~/lib/routers/baseRouter";

const prodBaseUrl = () => new URL(ApiUrl()).pathname;

const buildUrl = (path: string, params?: object) => {
    const query = params
        ? queryString.stringify(params, { skipNull: true })
        : "";

    if (
        process.env.NODE_ENV === "production" ||
        import.meta.env.VITE_API_IN_BROWSER === "true"
    ) {
        return {
            url: prodBaseUrl() + path + (query ? "?" + query : ""),
            path,
        };
    }

    const urlObj = new URL(ApiUrl() + path + (query ? "?" + query : ""));

    return {
        url: urlObj.toString(),
        path: urlObj.pathname,
    };
};

export const ApiRoutes = {
    assistants: {
        base: () => buildUrl("/assistants"),
        getAssistants: () => buildUrl("/assistants"),
        getCredentials: (assistantId: string) =>
            buildUrl(`/assistants/${assistantId}/credentials`),
        deleteCredential: (assistantId: string, credentialId: string) =>
            buildUrl(`/assistants/${assistantId}/credentials/${credentialId}`),
        getEvents: (assistantId: string) =>
            buildUrl(`/assistants/${assistantId}/events`),
        invoke: (assistantId: string) =>
            buildUrl(`/assistants/${assistantId}/invoke`),
        getTools: (assistantId: string) =>
            buildUrl(`/assistants/${assistantId}/tools`),
        deleteTool: (assistantId: string, toolId: string) =>
            buildUrl(`/assistants/${assistantId}/tools/${toolId}`),
        getFiles: (assistantId: string) =>
            buildUrl(`/assistants/${assistantId}/files`),
        getFileById: (assistantId: string, fileId: string) =>
            buildUrl(`/assistants/${assistantId}/files/${fileId}`),
        uploadFile: (assistantId: string) =>
            buildUrl(`/assistants/${assistantId}/files`),
        deleteFile: (assistantId: string, fileId: string) =>
            buildUrl(`/assistants/${assistantId}/files/${fileId}`),
        getKnowledge: (assistantId: string) =>
            buildUrl(`/assistants/${assistantId}/knowledge`),
        addKnowledge: (assistantId: string, fileName: string) =>
            buildUrl(`/assistants/${assistantId}/knowledge/${fileName}`),
        deleteKnowledge: (assistantId: string, fileName: string) =>
            buildUrl(`/assistants/${assistantId}/knowledge/${fileName}`),
    },
    agents: {
        base: () => buildUrl("/agents"),
        getById: (agentId: string) => buildUrl(`/agents/${agentId}`),
        getLocalKnowledgeFiles: (agentId: string) =>
            buildUrl(`/agents/${agentId}/knowledge-files`),
        addKnowledgeFiles: (agentId: string, fileName: string) =>
            buildUrl(`/agents/${agentId}/knowledge-files/${fileName}`),
        deleteKnowledgeFiles: (agentId: string, fileName: string) =>
            buildUrl(`/agents/${agentId}/knowledge-files/${fileName}`),
        createKnowledgeSource: (agentId: string) =>
            buildUrl(`/agents/${agentId}/knowledge-sources`),
        getKnowledgeSource: (agentId: string) =>
            buildUrl(`/agents/${agentId}/knowledge-sources`),
        updateKnowledgeSource: (agentId: string, knowledgeSourceId: string) =>
            buildUrl(
                `/agents/${agentId}/knowledge-sources/${knowledgeSourceId}`
            ),
        syncKnowledgeSource: (agentId: string, knowledgeSourceId: string) =>
            buildUrl(
                `/agents/${agentId}/knowledge-sources/${knowledgeSourceId}/sync`
            ),
        getAuthUrlForAgent: (agentId: string, toolRef: string) =>
            buildUrl(`/agents/${agentId}/oauth-credentials/${toolRef}/login`),
        deleteKnowledgeSource: (agentId: string, knowledgeSourceId: string) =>
            buildUrl(
                `/agents/${agentId}/knowledge-sources/${knowledgeSourceId}`
            ),
        getFilesForKnowledgeSource: (agentId: string, sourceId: string) =>
            buildUrl(
                `/agents/${agentId}/knowledge-sources/${sourceId}/knowledge-files`
            ),
        approveFile: (agentId: string, fileID: string) =>
            buildUrl(`/agents/${agentId}/approve-file/${fileID}`),
        reingestFile: (agentId: string, fileID: string, sourceId?: string) =>
            buildUrl(
                sourceId
                    ? `/agents/${agentId}/knowledge-sources/${sourceId}/knowledge-files/${fileID}/ingest`
                    : `/agents/${agentId}/knowledge-files/${fileID}/ingest`
            ),
    },
    workflows: {
        base: () => buildUrl("/workflows"),
        getById: (workflowId: string) => buildUrl(`/workflows/${workflowId}`),
        getKnowledge: (workflowId: string) =>
            buildUrl(`/workflows/${workflowId}/files`),
        addKnowledge: (workflowId: string, fileName: string) =>
            buildUrl(`/workflows/${workflowId}/files/${fileName}`),
        deleteKnowledge: (workflowId: string, fileName: string) =>
            buildUrl(`/workflows/${workflowId}/files/${fileName}`),
    },
    threads: {
        base: () => buildUrl("/threads"),
        getById: (threadId: string) => buildUrl(`/threads/${threadId}`),
        getByAgent: (agentId: string) => buildUrl(`/agents/${agentId}/threads`),
        events: (
            threadId: string,
            params?: {
                follow?: boolean;
                runID?: string;
                waitForThread?: boolean;
                maxRuns?: number;
            }
        ) => buildUrl(`/threads/${threadId}/events`, params),
        getKnowledge: (threadId: string) =>
            buildUrl(`/threads/${threadId}/knowledge`),
        getFiles: (threadId: string) => buildUrl(`/threads/${threadId}/files`),
    },
    prompt: {
        base: () => buildUrl("/prompt"),
        promptResponse: () => buildUrl("/prompt"),
    },
    runs: {
        base: () => buildUrl("/runs"),
        getRunById: (runId: string) => buildUrl(`/runs/${runId}`),
        getDebugById: (runId: string) => buildUrl(`/runs/${runId}/debug`),
        getByThread: (threadId: string) =>
            buildUrl(`/threads/${threadId}/runs`),
    },
    toolReferences: {
        base: (params?: { type?: ToolReferenceType }) =>
            buildUrl("/tool-references", params),
        getById: (toolReferenceId: string) =>
            buildUrl(`/tool-references/${toolReferenceId}`),
    },
    users: {
        base: () => buildUrl("/users"),
    },
    me: () => buildUrl("/me"),
    invoke: (id: string, threadId?: Nullish<string>) => {
        return threadId
            ? buildUrl(`/invoke/${id}/threads/${threadId}`)
            : buildUrl(`/invoke/${id}`);
    },
    oauthApps: {
        base: () => buildUrl("/oauth-apps"),
        getOauthApps: () => buildUrl("/oauth-apps"),
        createOauthApp: () => buildUrl(`/oauth-apps`),
        getOauthAppById: (id: string) => buildUrl(`/oauth-apps/${id}`),
        updateOauthApp: (id: string) => buildUrl(`/oauth-apps/${id}`),
        deleteOauthApp: (id: string) => buildUrl(`/oauth-apps/${id}`),
        supportedOauthAppTypes: () => buildUrl("/supported-oauth-app-types"),
        supportedAuthTypes: () => buildUrl("/supported-auth-types"),
    },
    models: {
        base: () => buildUrl("/models"),
        getModels: () => buildUrl("/models"),
        getModelById: (modelId: string) => buildUrl(`/models/${modelId}`),
        createModel: () => buildUrl(`/models`),
        updateModel: (modelId: string) => buildUrl(`/models/${modelId}`),
        deleteModel: (modelId: string) => buildUrl(`/models/${modelId}`),
    },
    defaultModelAliases: {
        base: () => buildUrl("/default-model-aliases"),
        getAliases: () => buildUrl("/default-model-aliases"),
        createAlias: () => buildUrl("/default-model-aliases"),
        getAliasById: (aliasId: string) =>
            buildUrl(`/default-model-aliases/${aliasId}`),
        updateAlias: (aliasId: string) =>
            buildUrl(`/default-model-aliases/${aliasId}`),
        deleteAlias: (aliasId: string) =>
            buildUrl(`/default-model-aliases/${aliasId}`),
    },
    webhooks: {
        base: () => buildUrl("/webhooks"),
        getWebhooks: () => buildUrl("/webhooks"),
        createWebhook: () => buildUrl(`/webhooks`),
        getWebhookById: (webhookId: string) =>
            buildUrl(`/webhooks/${webhookId}`),
        updateWebhook: (webhookId: string) =>
            buildUrl(`/webhooks/${webhookId}`),
        removeWebhookToken: (webhookId: string) =>
            buildUrl(`/webhooks/${webhookId}/remove-token`),
        deleteWebhook: (webhookId: string) =>
            buildUrl(`/webhooks/${webhookId}`),
        invoke: (webhookId: string) => buildUrl(`/webhooks/${webhookId}`),
    },
};

/** revalidates the cache for all routes that match the filter callback
 *
 * Standard format for setting up cache keys is { url: urlPath, ...restData }
 * where urlPath is the path of the api route
 */
export const revalidateWhere = async (filterCb: (url: string) => boolean) => {
    await mutate((key: unknown) => {
        if (
            key &&
            typeof key === "object" &&
            "url" in key &&
            typeof key.url === "string"
        ) {
            return filterCb(key.url);
        }
    });
};

import queryString from "query-string";
import { mutate } from "swr";

import { ToolReferenceType } from "~/lib/model/toolReferences";

// todo: We need to have a discussion on using https in dev
export const apiBaseUrl = "https://localhost:8080/api" as const;
const prodBaseUrl = new URL(apiBaseUrl).pathname;

const buildUrl = (path: string, params?: object) => {
    const query = params
        ? queryString.stringify(params, { skipNull: true })
        : "";

    if (
        process.env.NODE_ENV === "production" ||
        import.meta.env.VITE_API_IN_BROWSER === "true"
    ) {
        return {
            url: prodBaseUrl + path + (query ? "?" + query : ""),
            path,
        };
    }

    const urlObj = new URL(apiBaseUrl + path + (query ? "?" + query : ""));

    return {
        url: urlObj.toString(),
        path: urlObj.pathname,
    };
};

export const ApiRoutes = {
    agents: {
        base: () => buildUrl("/agents"),
        getById: (agentId: string) => buildUrl(`/agents/${agentId}`),
        getKnowledge: (agentId: string) =>
            buildUrl(`/agents/${agentId}/knowledge`),
        addKnowledge: (agentId: string, fileName: string) =>
            buildUrl(`/agents/${agentId}/knowledge/${fileName}`),
        deleteKnowledge: (agentId: string, fileName: string) =>
            buildUrl(`/agents/${agentId}/knowledge/${fileName}`),
        triggerKnowledgeIngestion: (agentId: string) =>
            buildUrl(`/agents/${agentId}/knowledge`),
        createRemoteKnowledgeSource: (agentId: string) =>
            buildUrl(`/agents/${agentId}/remote-knowledge-sources`),
        getRemoteKnowledgeSource: (agentId: string) =>
            buildUrl(`/agents/${agentId}/remote-knowledge-sources`),
        updateRemoteKnowledgeSource: (
            agentId: string,
            remoteKnowledgeSourceId: string
        ) =>
            buildUrl(
                `/agents/${agentId}/remote-knowledge-sources/${remoteKnowledgeSourceId}`
            ),
        deleteRemoteKnowledgeSource: (
            agentId: string,
            remoteKnowledgeSourceId: string
        ) =>
            buildUrl(
                `/agents/${agentId}/remote-knowledge-sources/${remoteKnowledgeSourceId}`
            ),
        approveKnowledgeFile: (agentId: string, fileID: string) =>
            buildUrl(`/agents/${agentId}/knowledge/${fileID}/approve`),
    },
    workflows: {
        base: () => buildUrl("/workflows"),
        getById: (workflowId: string) => buildUrl(`/workflows/${workflowId}`),
        getKnowledge: (workflowId: string) =>
            buildUrl(`/workflows/${workflowId}/knowledge`),
        addKnowledge: (workflowId: string, fileName: string) =>
            buildUrl(`/workflows/${workflowId}/knowledge/${fileName}`),
        deleteKnowledge: (workflowId: string, fileName: string) =>
            buildUrl(`/workflows/${workflowId}/knowledge/${fileName}`),
    },
    threads: {
        base: () => buildUrl("/threads"),
        getById: (threadId: string) => buildUrl(`/threads/${threadId}`),
        getByAgent: (agentId: string) => buildUrl(`/agents/${agentId}/threads`),
        events: (
            threadId: string,
            params?: { follow?: boolean; runID?: string }
        ) => buildUrl(`/threads/${threadId}/events`, params),
        getKnowledge: (threadId: string) =>
            buildUrl(`/threads/${threadId}/knowledge`),
        getFiles: (threadId: string) => buildUrl(`/threads/${threadId}/files`),
    },
    runs: {
        base: () => buildUrl("/runs"),
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

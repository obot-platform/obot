import queryString from "query-string";
import { mutate } from "swr";

import { AssistantNamespace } from "~/lib/model/assistants";
import { CredentialNamespace } from "~/lib/model/credentials";
import {
	KnowledgeFileNamespace,
	KnowledgeSourceNamespace,
} from "~/lib/model/knowledge";
import { ToolReferenceType } from "~/lib/model/toolReferences";
import { ApiUrl } from "~/lib/routers/baseRouter";

import { TableNamespace } from "~/components/model/tables";

const prodBaseUrl = () => new URL(ApiUrl()).pathname;

const buildUrl = (path: string, params?: object) => {
	const query = params ? queryString.stringify(params, { skipNull: true }) : "";

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
		getAssistants: () => buildUrl("/assistants"),
	},
	knowledgeSources: {
		getKnowledgeSources: (
			namespace: KnowledgeSourceNamespace,
			entityId: string
		) => buildUrl(`/${namespace}/${entityId}/knowledge-sources`),
		createKnowledgeSource: (
			namespace: KnowledgeSourceNamespace,
			entityId: string
		) => buildUrl(`/${namespace}/${entityId}/knowledge-sources`),
		getKnowledgeSource: (
			namespace: KnowledgeSourceNamespace,
			entityId: string,
			sourceId: string
		) => buildUrl(`/${namespace}/${entityId}/knowledge-sources/${sourceId}`),
		updateKnowledgeSource: (
			namespace: KnowledgeSourceNamespace,
			entityId: string,
			sourceId: string
		) => buildUrl(`/${namespace}/${entityId}/knowledge-sources/${sourceId}`),
		deleteKnowledgeSource: (
			namespace: KnowledgeSourceNamespace,
			entityId: string,
			sourceId: string
		) => buildUrl(`/${namespace}/${entityId}/knowledge-sources/${sourceId}`),
		syncKnowledgeSource: (
			namespace: KnowledgeSourceNamespace,
			entityId: string,
			sourceId: string
		) =>
			buildUrl(`/${namespace}/${entityId}/knowledge-sources/${sourceId}/sync`),
		getFilesForKnowledgeSource: (
			namespace: KnowledgeSourceNamespace,
			entityId: string,
			sourceId: string
		) =>
			buildUrl(
				`/${namespace}/${entityId}/knowledge-sources/${sourceId}/knowledge-files`
			),
		reingestKnowledgeFileFromSource: (
			namespace: KnowledgeSourceNamespace,
			entityId: string,
			sourceId: string,
			fileName: string
		) =>
			buildUrl(
				`/${namespace}/${entityId}/knowledge-sources/${sourceId}/knowledge-files/${fileName}/ingest`
			),
		approveFile: (
			namespace: KnowledgeSourceNamespace,
			entityId: string,
			fileName: string
		) => buildUrl(`/${namespace}/${entityId}/approve-file/${fileName}`),
		watchKnowledgeSourceFiles: (
			namespace: KnowledgeSourceNamespace,
			entityId: string,
			sourceId: string
		) =>
			buildUrl(
				`/${namespace}/${entityId}/knowledge-sources/${sourceId}/knowledge-files/watch`
			),
	},
	knowledgeFiles: {
		getKnowledgeFiles: (namespace: KnowledgeFileNamespace, entityId: string) =>
			buildUrl(`/${namespace}/${entityId}/knowledge-files`),
		getKnowledgeFileById: (
			namespace: KnowledgeFileNamespace,
			entityId: string,
			fileName: string
		) => buildUrl(`/${namespace}/${entityId}/knowledge-files/${fileName}`),
		addKnowledgeFile: (
			namespace: KnowledgeFileNamespace,
			entityId: string,
			fileName: string
		) => buildUrl(`/${namespace}/${entityId}/knowledge-files/${fileName}`),
		updateKnowledgeFile: (
			namespace: KnowledgeFileNamespace,
			entityId: string,
			fileName: string
		) => buildUrl(`/${namespace}/${entityId}/knowledge-files/${fileName}`),
		deleteKnowledgeFile: (
			namespace: KnowledgeFileNamespace,
			entityId: string,
			fileName: string
		) => buildUrl(`/${namespace}/${entityId}/knowledge-files/${fileName}`),
		reingestKnowledgeFile: (
			namespace: KnowledgeFileNamespace,
			entityId: string,
			fileName: string
		) =>
			buildUrl(`/${namespace}/${entityId}/knowledge-files/${fileName}/ingest`),
	},
	agents: {
		base: () => buildUrl("/agents"),
		getById: (agentId: string) => buildUrl(`/agents/${agentId}`),
		getAuthUrlForAgent: (agentId: string, toolRef: string) =>
			buildUrl(`/agents/${agentId}/oauth-credentials/${toolRef}/login`),
		getAuthorizations: (agentId: string) =>
			buildUrl(`/agents/${agentId}/authorizations`),
		addAuthorization: (agentId: string) =>
			buildUrl(`/agents/${agentId}/authorizations/add`),
		removeAuthorization: (agentId: string) =>
			buildUrl(`/agents/${agentId}/authorizations/remove`),
		getWorkspaceFiles: (agentId: string) =>
			buildUrl(`/agents/${agentId}/files`),
		getWorkspaceFile: (agentId: string, fileName: string) =>
			buildUrl(`/agents/${agentId}/file/${fileName}`),
		removeWorkspaceFile: (agentId: string, fileName: string) =>
			buildUrl(`/agents/${agentId}/file/${fileName}`),
		uploadWorkspaceFile: (agentId: string, fileName: string) =>
			buildUrl(`/agents/${agentId}/file/${fileName}`),
		getThreadsForAgent: (agentId: string) =>
			buildUrl(`/agents/${agentId}/threads`),
		setDefault: (agentId: string) => buildUrl(`/agents/${agentId}/setdefault`),
	},
	tasks: {
		base: () => buildUrl("/tasks"),
		getById: (taskId: string) => buildUrl(`/tasks/${taskId}`),
		authenticate: (taskId: string) => buildUrl(`/tasks/${taskId}/authenticate`),
		deleteWithTriggers: (taskId: string) => buildUrl(`/tasks/${taskId}`),
	},
	toolAuthentication: {
		authenticate: (namespace: AssistantNamespace, entityId: string) =>
			buildUrl(`/${namespace}/${entityId}/authenticate`),
		deauthenticate: (namespace: AssistantNamespace, entityId: string) =>
			buildUrl(`/${namespace}/${entityId}/deauthenticate`),
	},
	env: {
		getEnv: (entityId: string) => buildUrl(`/agents/${entityId}/env`),
		updateEnv: (entityId: string) => buildUrl(`/agents/${entityId}/env`),
	},
	credentials: {
		getCredentials: (namespace: CredentialNamespace, entityId: string) =>
			buildUrl(`/${namespace}/${entityId}/credentials`),
		deleteCredential: (
			namespace: CredentialNamespace,
			entityId: string,
			credentialId: string
		) => buildUrl(`/${namespace}/${entityId}/credentials/${credentialId}`),
	},
	threads: {
		base: () => buildUrl("/threads"),
		getById: (threadId: string) => buildUrl(`/threads/${threadId}`),
		updateById: (threadId: string) => buildUrl(`/threads/${threadId}`),
		getByAgent: (agentId: string) => buildUrl(`/agents/${agentId}/threads`),
		getTasksForThread: (threadId: string) =>
			buildUrl(`/threads/${threadId}/tasks`),
		events: (
			threadId: string,
			params?: {
				follow?: boolean;
				runID?: string;
				waitForThread?: boolean;
				maxRuns?: number;
			}
		) => buildUrl(`/threads/${threadId}/events`, params),
		getFiles: (threadId: string) => buildUrl(`/threads/${threadId}/files`),
		abortById: (threadId: string) => buildUrl(`/threads/${threadId}/abort`),
		downloadFile: (threadId: string, filePath: string) =>
			buildUrl(`/threads/${threadId}/file/${filePath}`),
	},
	prompt: {
		base: () => buildUrl("/prompt"),
		promptResponse: () => buildUrl("/prompt"),
	},
	projects: {
		getAll: () => buildUrl("/projects", { all: true }),
		getById: (projectId: string) => buildUrl(`/projects/${projectId}`),
		deleteProject: (agentId: string, projectId: string) =>
			buildUrl(`/assistants/${agentId}/projects/${projectId}`),
	},
	projectShares: {
		getAll: () => buildUrl("/shares", { all: true }),
	},
	runs: {
		base: () => buildUrl("/runs"),
		getRunById: (runId: string) => buildUrl(`/runs/${runId}`),
		getDebugById: (runId: string) => buildUrl(`/runs/${runId}/debug`),
		getByThread: (threadId: string) => buildUrl(`/threads/${threadId}/runs`),
	},
	toolReferences: {
		base: (params?: { type?: ToolReferenceType }) =>
			buildUrl("/tool-references", params),
		getById: (toolReferenceId: string) =>
			buildUrl(`/tool-references/${toolReferenceId}`),
		purgeCache: (toolReferenceId: string) =>
			buildUrl(`/tool-references/${toolReferenceId}/force-refresh`),
	},
	users: {
		base: () => buildUrl("/users"),
		updateUser: (username: string) => buildUrl(`/users/${username}`),
		deleteUser: (username: string) => buildUrl(`/users/${username}`),
		getOne: (username: string) =>
			buildUrl(`/users/${username}`, { "by-id": true }),
	},
	workspace: {
		getTables: (namespace: TableNamespace, entityId: string) =>
			buildUrl(`/${namespace}/${entityId}/tables`),
		getTableRows: (
			namespace: TableNamespace,
			entityId: string,
			tableName: string
		) => buildUrl(`/${namespace}/${entityId}/tables/${tableName}/rows`),
	},
	me: () => buildUrl("/me"),
	invoke: (
		id: string,
		threadId?: Nullish<string>,
		params?: { async?: boolean }
	) => {
		return threadId
			? buildUrl(`/invoke/${id}/threads/${threadId}`, params)
			: buildUrl(`/invoke/${id}`, params);
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
		getAvailableModels: () => buildUrl("/available-models"),
		getAvailableModelsByProvider: (provider: string) =>
			buildUrl(`/available-models/${provider}`),
	},
	modelProviders: {
		base: () => buildUrl("/model-providers"),
		getModelProviders: () => buildUrl("/model-providers"),
		getModelProviderById: (modelProviderKey: string) =>
			buildUrl(`/model-providers/${modelProviderKey}`),
		validateModelProviderById: (modelProviderKey: string) =>
			buildUrl(`/model-providers/${modelProviderKey}/validate`),
		configureModelProviderById: (modelProviderKey: string) =>
			buildUrl(`/model-providers/${modelProviderKey}/configure`),
		revealModelProviderById: (modelProviderKey: string) =>
			buildUrl(`/model-providers/${modelProviderKey}/reveal`),
		deconfigureModelProviderById: (modelProviderKey: string) =>
			buildUrl(`/model-providers/${modelProviderKey}/deconfigure`),
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
		getWebhookById: (webhookId: string) => buildUrl(`/webhooks/${webhookId}`),
		updateWebhook: (webhookId: string) => buildUrl(`/webhooks/${webhookId}`),
		removeWebhookToken: (webhookId: string) =>
			buildUrl(`/webhooks/${webhookId}/remove-token`),
		deleteWebhook: (webhookId: string) => buildUrl(`/webhooks/${webhookId}`),
		invoke: (webhookId: string) => buildUrl(`/webhooks/${webhookId}`),
	},
	cronjobs: {
		getCronJobs: () => buildUrl("/cronjobs"),
		getCronJobById: (cronJobId: string) => buildUrl(`/cronjobs/${cronJobId}`),
		createCronJob: () => buildUrl("/cronjobs"),
		deleteCronJob: (cronJobId: string) => buildUrl(`/cronjobs/${cronJobId}`),
		updateCronJob: (cronJobId: string) => buildUrl(`/cronjobs/${cronJobId}`),
	},
	emailReceivers: {
		getEmailReceivers: () => buildUrl("/email-receivers"),
		getEmailReceiverById: (id: string) => buildUrl(`/email-receivers/${id}`),
		createEmailReceiver: () => buildUrl(`/email-receivers`),
		updateEmailReceiver: (id: string) => buildUrl(`/email-receivers/${id}`),
		deleteEmailReceiver: (id: string) => buildUrl(`/email-receivers/${id}`),
	},
	version: () => buildUrl("/version"),
	authProviders: {
		base: () => buildUrl("/auth-providers"),
		getAuthProviders: () => buildUrl("/auth-providers"),
		getAuthProviderById: (authProviderId: string) =>
			buildUrl(`/auth-providers/${authProviderId}`),
		configureAuthProviderById: (authProviderId: string) =>
			buildUrl(`/auth-providers/${authProviderId}/configure`),
		revealAuthProviderById: (authProviderId: string) =>
			buildUrl(`/auth-providers/${authProviderId}/reveal`),
		deconfigureAuthProviderById: (authProviderId: string) =>
			buildUrl(`/auth-providers/${authProviderId}/deconfigure`), // TODO - implement this in the backend
	},
	bootstrap: {
		status: () => buildUrl("/bootstrap"),
		login: () => buildUrl("/bootstrap/login"),
		logout: () => buildUrl("/bootstrap/logout"),
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
		} else {
			console.error("Invalid cache key", key);
			return false;
		}
	});
};

export const createRevalidate = <
	TParams extends unknown[],
	TReturn extends { path: string },
>(
	fn: (...params: TParams) => TReturn
) => {
	return (...params: TParams) => {
		const re = new RegExp(fn(...params).path);
		return revalidateWhere((url) => re.test(url));
	};
};

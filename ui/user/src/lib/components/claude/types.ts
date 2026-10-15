import type { AccessControlRuleSubject } from '$lib/services';

export type ClaudeResources = {
	actorCPU?: string;
	actorMemory?: string;
	idleTimeoutMinutes?: number;
	sessionRetentionDays?: number;
};
export type EgressSecret = {
	id: string;
	hostname: string;
	header: string;
	value?: string;
};
export type EnvironmentProfile = {
	egressSecrets?: EgressSecret[];
	name: string;
	subjects: AccessControlRuleSubject[];
	mcpServerIDs: string[] | null;
	egressHosts?: string[];
	allowAllEgress?: boolean;
	resources: ClaudeResources;
};
export type ClaudeRuntime = {
	managedSettings?: Record<string, unknown>;
	image: string;
	workerImage: string;
	allowAllEgress?: boolean;
	egressHosts: string[];
	worker: {
		workerReplicas: number;
		cpuLimit: string;
		memoryLimit: string;
	};
	resources: ClaudeResources;
};
export type ClaudeEnvironment = {
	egressSecrets?: EgressSecret[];
	id: string;
	displayName: string;
	organizationID: string;
	environmentID: string;
	profiles: EnvironmentProfile[];
	runtime: ClaudeRuntime;
	claudeTagUserID?: string;
	claudeTagEnabled: boolean;
};
export type ClaudeOrganizationAccount = {
	organizationID: string;
	account: string;
	uuid: string;
	displayName: string;
	email: string;
	userID: string;
};
export type ClaudeConfig = {
	enabled: boolean;
	environments?: ClaudeEnvironment[];
	mappings?: ClaudeOrganizationAccount[];
	defaultWorkerImage?: string;
	defaultRunnerImage?: string;
};

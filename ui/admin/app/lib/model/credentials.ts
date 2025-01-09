export const CredentialNamespace = {
	Threads: "threads",
	Agents: "agents",
	Workflows: "workflows",
} as const;
export type CredentialNamespace =
	(typeof CredentialNamespace)[keyof typeof CredentialNamespace];

export type Credential = {
	contextID: string;
	name: string;
	envVars: string[];
	expiresAt?: string; // date
};

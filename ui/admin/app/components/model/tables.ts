export const TableNamespace = {
	Threads: "threads",
} as const;
export type TableNamespace =
	(typeof TableNamespace)[keyof typeof TableNamespace];

export type WorkspaceTable = { name: string };

export type WorkspaceTableRows = {
	columns?: string[];
	rows: Record<string, string>[];
};

export const CapabilityTool = {
	Knowledge: 'knowledge',
	WorkspaceFiles: 'workspace-files',
	Database: 'database',
	Tasks: 'tasks',
	Projects: 'projects',
	Threads: 'threads'
} as const;
export type CapabilityTool = (typeof CapabilityTool)[keyof typeof CapabilityTool];

export const isCapabilityTool = (value: string): value is CapabilityTool =>
	Object.values(CapabilityTool).includes(value as CapabilityTool);

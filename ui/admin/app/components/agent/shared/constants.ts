import { CapabilityTool } from "~/lib/model/toolReferences";

// use record to allow linter to check for missing keys
const CapabilityToolOrder = {
	[CapabilityTool.Knowledge]: 0,
	[CapabilityTool.WorkspaceFiles]: 1,
	[CapabilityTool.Tasks]: 2,
	[CapabilityTool.Projects]: 3,
	[CapabilityTool.Threads]: 4,
	[CapabilityTool.Memory]: 5,
} satisfies Record<CapabilityTool, number>;

export const getCapabilityToolOrder = (tool: string) => {
	if (tool in CapabilityToolOrder) {
		return CapabilityToolOrder[tool as CapabilityTool];
	}

	return 999;
};

import { CapabilityTool } from "~/lib/model/toolReferences";

// use record to allow linter to check for missing keys
const CapabilityToolOrder = {
	[CapabilityTool.Knowledge]: 0,
	[CapabilityTool.WorkspaceFiles]: 1,
	[CapabilityTool.Database]: 2,
	[CapabilityTool.Tasks]: 3,
	[CapabilityTool.Projects]: 4,
	[CapabilityTool.Threads]: 5,
	[CapabilityTool.Memory]: 6,
} satisfies Record<CapabilityTool, number>;

export const getCapabilityToolOrder = (tool: string) => {
	if (tool in CapabilityToolOrder) {
		return CapabilityToolOrder[tool as CapabilityTool];
	}

	return 999;
};

export type TopToolCallRow = {
	compositeKey: string;
	toolLabel: string;
	count: number;
	serverDisplayName: string;
};

export type TopServerUsageRow = { serverName: string; count: number };

export type AvgToolCallResponseTimeRow = {
	toolName: string;
	averageResponseTimeMs: number;
	serverDisplayName: string;
};

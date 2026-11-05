import type { DeviceScanTopBucket } from '$lib/components/admin/device-scan/deviceScanTopBuckets';
import type { DonutDatum } from '$lib/components/graph/DonutGraph.svelte';
import type { MCPCatalogEntry, MCPCatalogServer } from '$lib/services';
import type {
	AvgToolCallResponseTimeRow,
	TopServerUsageRow,
	TopToolCallRow
} from '$lib/services/dashboard/types';
import type { DeviceStatId, PlatformStatId } from '../dashboardLayout';
import type { Component } from 'svelte';

export type DashboardStatTile<Id extends string> = {
	id: Id;
	label: string;
	loading: boolean;
	value: number;
	icon: Component<{ class?: string }>;
	seeMore?: string;
	format?: (value: number) => string;
};

export type PopularServer = {
	entry?: MCPCatalogEntry;
	server?: MCPCatalogServer;
	count: number;
	id: string;
};

export type DashboardWidgetModel = {
	isBootStrapUser: boolean;
	hasDeviceScans: boolean;
	start: Date;
	end: Date;
	platformTiles: DashboardStatTile<PlatformStatId>[];
	deviceTiles: DashboardStatTile<DeviceStatId>[];
	serverLoading: boolean;
	doesSupportK8sUpdates: boolean;
	deploymentStatusBreakdown: { status: string; count: number }[];
	totalServers: number;
	graphData: DonutDatum[];
	loadingToolUsage: boolean;
	topServerUsage: TopServerUsageRow[];
	maxServersToShow: number;
	popularServers: PopularServer[];
	serversLoading: boolean;
	topToolCalls: TopToolCallRow[];
	maxToolsToShow: number;
	avgToolCallResponseTime: AvgToolCallResponseTimeRow[];
	loadingDeviceScanStats: boolean;
	deviceScanSkillBuckets: DeviceScanTopBucket[];
	totalDeviceScanSkillGroups: number;
	deviceScanClientBuckets: DeviceScanTopBucket[];
	totalDeviceScanClientGroups: number;
	deviceScanMcpBuckets: DeviceScanTopBucket[];
	totalDeviceScanMcpGroups: number;
	deviceScanTimelineRows: { scanned_at: string; category: 'scans' }[];
	totalDeviceScanSubmissions: number;
};

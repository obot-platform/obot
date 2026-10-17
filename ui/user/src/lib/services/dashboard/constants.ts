import type { DonutLegendItem } from '$lib/components/graph/DonutGraph.svelte';
import { CATEGORICAL_LIGHT } from '$lib/components/graph/palette';
import { m } from '$lib/i18n';

export const DEPLOYMENT_STATUS_ORDER = [
	'Available',
	'Progressing',
	'Unavailable',
	'Needs Attention',
	'Shutdown',
	'Unknown'
] as const;

export const ENTRY_TYPE_GRAPH_META: {
	key: 'single' | 'multi' | 'local' | 'remote';
	label: string;
	baseColor: string;
}[] = [
	{ key: 'single', label: m.dashboard_entry_type_hosted_single(), baseColor: CATEGORICAL_LIGHT[2] },
	{ key: 'multi', label: m.dashboard_entry_type_hosted_multi(), baseColor: CATEGORICAL_LIGHT[1] },
	{ key: 'remote', label: m.dashboard_entry_type_remote(), baseColor: CATEGORICAL_LIGHT[0] }
];

export const entryTypeDonutLegend: DonutLegendItem[] = ENTRY_TYPE_GRAPH_META.map(
	({ label, baseColor }) => ({ label, color: baseColor })
);

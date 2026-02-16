<script lang="ts">
	import type { AuditLog } from '$lib/services';
	import StackedBarsChart from '$lib/components/charts/StackedBarsChart.svelte';

	interface Props {
		start: Date;
		end: Date;
		data: AuditLog[];
	}

	let { start, end, data }: Props = $props();

	const colorByCallType: Record<string, string> = {
		initialize: '#254993',
		'notifications/initialized': '#D65C7C',
		'notifications/message': '#635DB6',
		'prompts/list': '#D6A95C',
		'resources/list': '#2EB88A',
		'tools/call': '#47A3D1',
		'tools/list': '#D0CE43'
	};
</script>

<StackedBarsChart
	{start}
	{end}
	{data}
	dateAccessor={(item) => item.createdAt}
	categoryAccessor={(item) => item.callType}
	colorScheme={colorByCallType}
/>

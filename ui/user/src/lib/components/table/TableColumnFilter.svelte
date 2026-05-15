<script lang="ts">
	import { popover } from '$lib/actions';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Select from '../Select.svelte';
	import { Columns3Cog } from 'lucide-svelte';

	interface Props {
		disablePortal?: boolean;
		fields: string[];
		headers?: { title: string; property: string }[];
		hiddenFieldIndices: Set<number>;
		onVisibilityChange?: (hiddenIndices: Set<number>) => void;
		onReset?: () => void;
		showReset?: boolean;
	}

	let {
		disablePortal = false,
		fields,
		headers,
		hiddenFieldIndices,
		onReset,
		onVisibilityChange,
		showReset
	}: Props = $props();

	function getFieldLabel(field: string): string {
		const header = headers?.find((h) => h.property === field);
		if (header?.title) return header.title;
		return field.charAt(0).toUpperCase() + field.slice(1);
	}

	const {
		tooltip: tooltipRef,
		ref,
		toggle
	} = popover({
		placement: 'bottom-start'
	});

	function handleVisibilityChange(selectedFieldIds: string[]) {
		// eslint-disable-next-line svelte/prefer-svelte-reactivity
		const newHiddenIndices = new Set<number>();
		fields.forEach((field, index) => {
			if (!selectedFieldIds.includes(field)) {
				newHiddenIndices.add(index);
			}
		});
		onVisibilityChange?.(newHiddenIndices);
	}
</script>

<button
	use:ref
	class="flex grow items-center px-2 py-3"
	use:tooltip={{ disablePortal, text: 'Filter columns', classes: ['z-60'] }}
	onclick={() => toggle()}
>
	<Columns3Cog class="size-4 shrink-0" />
</button>
<div use:tooltipRef={{ disablePortal }} class="popover w-xs rounded-xs">
	<Select
		class="rounded-xs border border-transparent font-normal shadow-inner"
		classes={{
			root: 'flex grow',
			option: 'font-normal dark:bg-base-300 bg-base-100'
		}}
		options={fields.map((f) => ({
			label: getFieldLabel(f),
			id: f
		}))}
		onClear={(_option, value) => {
			if (typeof value === 'string') {
				const selectedFieldIds = value.split(',').filter(Boolean);
				if (selectedFieldIds.length === 0) {
					onReset?.();
				} else {
					handleVisibilityChange(selectedFieldIds);
				}
			}
		}}
		onSelect={(_option, value) => {
			if (typeof value !== 'string') return;
			const selectedFieldIds = value.split(',').filter(Boolean);

			if (selectedFieldIds.length === 0) {
				onReset?.();
			} else {
				handleVisibilityChange(selectedFieldIds);
			}
		}}
		multiple
		selected={fields.filter((_f, index) => !hiddenFieldIndices.has(index)).join(',')}
		placeholder="Filter columns..."
		onClearAll={showReset ? onReset : undefined}
		clearAllLabel="Reset"
	/>
</div>

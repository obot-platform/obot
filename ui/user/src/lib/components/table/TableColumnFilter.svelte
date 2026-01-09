<script lang="ts">
	import { popover } from '$lib/actions';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { Columns3Cog } from 'lucide-svelte';
	import Select from '../Select.svelte';

	interface Props {
		disablePortal?: boolean;
		fields: string[];
		hiddenFieldIndices: Set<number>;
		onVisibilityChange?: (hiddenIndices: Set<number>) => void;
		onReset?: () => void;
		showReset?: boolean;
	}

	let {
		disablePortal = false,
		fields,
		hiddenFieldIndices,
		onReset,
		onVisibilityChange,
		showReset
	}: Props = $props();

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
	use:tooltip={'Filter columns'}
	onclick={() => toggle()}
>
	<Columns3Cog class="size-4 flex-shrink-0" />
</button>
<div use:tooltipRef={{ disablePortal }} class="default-dialog w-xs rounded-xs">
	<Select
		class="rounded-xs border border-transparent shadow-inner"
		classes={{
			root: 'flex grow'
		}}
		options={fields.map((f) => ({
			label: f,
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
			if (typeof value === 'string') {
				const selectedFieldIds = value.split(',').filter(Boolean);
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

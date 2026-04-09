<script lang="ts">
	import Select from '../Select.svelte';
	import { SvelteMap } from 'svelte/reactivity';

	type Props = {
		categories?: string;
		readonly?: boolean;
		options?: { id: string; label: string }[];
		onCreate?: (value: string) => void;
		onUpdate?: (value: string) => void;
		onDelete?: (option: { id: string; label: string }) => void;
	};

	let {
		categories = $bindable(),
		readonly = false,
		options = [],
		onCreate,
		onUpdate,
		onDelete
	}: Props = $props();

	let optionsMap = $derived(
		new SvelteMap<string, { id: string; label: string }>(
			options.map((option) => [option.id, option])
		)
	);

	let localOptions = $derived([...optionsMap.values()]);

	let query = $state('');
</script>

<div class="category-select-input flex w-full items-center gap-2">
	<Select
		class="dark:border-surface3 bg-surface1 text-input-filled dark:bg-background border border-transparent shadow-inner"
		classes={{
			root: 'w-full',
			clear: 'hover:bg-surface3 bg-transparent'
		}}
		options={localOptions}
		disabled={readonly}
		placeholder="Type to Search for a category | hit &quot;Enter&quot; to create one"
		bind:query
		bind:selected={
			() => categories,
			(v) => {
				categories = v;
			}
		}
		multiple
		onSelect={(_, value) => {
			onUpdate?.(value as string);
		}}
		onClear={(option, value) => {
			onUpdate?.(value as string);
			if (option) {
				onDelete?.(option);
			}
		}}
		onKeyDown={(ev, params) => {
			const { results } = params ?? {};
			if (!results?.length) {
				if (ev.key === 'Enter') {
					ev.preventDefault();
					optionsMap.set(query, { label: query, id: query });
					onCreate?.(query);
					query = '';
				}
			}
		}}
	/>
</div>

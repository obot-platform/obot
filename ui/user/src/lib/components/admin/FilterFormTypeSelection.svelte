<script lang="ts">
	import {
		PII_BLOCK_TYPES,
		PII_FILTER_DEFAULT_OPTIONS,
		PII_FILTER_OPTION_VALUES,
		PII_FILTER_OPTIONAL_OPTIONS,
		PII_REDACT_TYPES
	} from '$lib/constants';
	import type { MCPCatalogEntryFieldManifest } from '$lib/services';
	import { randomUUID } from '$lib/utils';
	import Select from '../Select.svelte';
	import { Plus, Trash2 } from 'lucide-svelte';

	interface Props {
		config: MCPCatalogEntryFieldManifest[];
	}

	let { config = $bindable() }: Props = $props();
	let blockedTypes = $derived(
		(config.find((env) => env.key === PII_BLOCK_TYPES)?.value ?? '')
			.split(',')
			.filter((type) => type.trim())
	);
	let redactedTypes = $derived(
		(config.find((env) => env.key === PII_REDACT_TYPES)?.value ?? '')
			.split(',')
			.filter((type) => type.trim())
	);
	let customOptions = $derived(
		[...blockedTypes, ...redactedTypes].filter(
			(type) => !PII_FILTER_DEFAULT_OPTIONS.some((option) => option.id === type)
		)
	);
	let unassignedCustomOptions = $state<{ id: string; key: string; value: string }[]>([]);

	function writeBlockTypes(ids: string[]) {
		const idx = config.findIndex((env) => env.key === PII_BLOCK_TYPES);
		if (idx !== -1) config[idx].value = ids.join(',');
	}

	function writeMutateTypes(ids: string[]) {
		const idx = config.findIndex((env) => env.key === PII_REDACT_TYPES);
		if (idx !== -1) config[idx].value = ids.join(',');
	}

	function applyPiiPolicy(typeId: string, policy: 'block' | 'redact' | 'none') {
		if (policy === 'block') {
			writeBlockTypes([...blockedTypes, typeId]);
			writeMutateTypes(redactedTypes.filter((t) => t !== typeId));
		} else if (policy === 'redact') {
			writeMutateTypes([...redactedTypes, typeId]);
			writeBlockTypes(blockedTypes.filter((t) => t !== typeId));
		} else {
			writeBlockTypes(blockedTypes.filter((t) => t !== typeId));
			writeMutateTypes(redactedTypes.filter((t) => t !== typeId));
		}
	}

	function removeTypeFromOneList(typeId: string, list: 'block' | 'mutate') {
		if (list === 'block') {
			writeBlockTypes(blockedTypes.filter((t) => t !== typeId));
		} else {
			writeMutateTypes(redactedTypes.filter((t) => t !== typeId));
		}
	}

	function tryCommitUnassignedRow(rowIndex: number) {
		const row = unassignedCustomOptions[rowIndex];
		if (!row?.key || (row.value !== 'block' && row.value !== 'redact')) return;
		applyPiiPolicy(row.key, row.value);
		unassignedCustomOptions.splice(rowIndex, 1);
	}
</script>

<!-- <div class="notification-info p-3 text-sm font-light">
	<p class="text-sm font-light break-all">{config.description}</p>
</div> -->

<div class="flex flex-col md:gap-2 gap-4 w-full">
	{#each PII_FILTER_DEFAULT_OPTIONS as option (option.id)}
		{@const isBlocked = blockedTypes.includes(option.id)}
		{@const isRedacted = redactedTypes.includes(option.id)}
		<div class="w-full flex-col md:flex-row flex md:items-center md:justify-between md:gap-4 gap-1">
			<p class="font-light capitalize">
				{option.label}
			</p>
			<Select
				classes={{
					root: 'w-full md:w-96'
				}}
				class="bg-surface1 shadow-inner! dark:bg-background dark:border-surface3 border border-transparent"
				options={PII_FILTER_OPTION_VALUES}
				selected={isBlocked ? 'block' : isRedacted ? 'redact' : 'none'}
				id={`pii-filter-type-${option.id}`}
				onSelect={(selected) => {
					const id = selected.id;
					if (id === 'block' || id === 'redact' || id === 'none') {
						applyPiiPolicy(option.id, id);
					}
				}}
			/>
		</div>
	{/each}
	{#each customOptions as option (option)}
		{@const isBlocked = blockedTypes.includes(option)}
		{@const isRedacted = redactedTypes.includes(option)}
		<div class="w-full flex-col md:flex-row flex md:items-center md:justify-between md:gap-4 gap-1">
			<Select
				classes={{ root: 'flex grow' }}
				class="bg-surface1 shadow-inner! dark:bg-background dark:border-surface3 border border-transparent"
				options={PII_FILTER_OPTIONAL_OPTIONS}
				selected={option}
				id={`pii-filter-type-${option}-selector`}
				placeholder="Select filter type..."
				searchInDropdown
			/>
			<Select
				classes={{
					root: 'w-full md:w-82'
				}}
				class="bg-surface1 shadow-inner! dark:bg-background dark:border-surface3 border border-transparent"
				options={PII_FILTER_OPTION_VALUES}
				selected={isBlocked ? 'block' : isRedacted ? 'redact' : 'none'}
				id={`pii-filter-type-${option}`}
				onSelect={(selected) => {
					const id = selected.id;
					if (id === 'none') {
						unassignedCustomOptions.push({ id: randomUUID(), key: option, value: 'none' });
					}
					if (id === 'block' || id === 'redact' || id === 'none') {
						applyPiiPolicy(option, id);
					}
				}}
			/>
			<button
				class="icon-button hover:text-red-500"
				onclick={() => {
					if (isBlocked) removeTypeFromOneList(option, 'block');
					else if (isRedacted) removeTypeFromOneList(option, 'mutate');
				}}
			>
				<Trash2 class="size-4" />
			</button>
		</div>
	{/each}
	{#each unassignedCustomOptions as option, i (option.id)}
		<div class="w-full flex-col md:flex-row flex md:items-center md:justify-between md:gap-4 gap-1">
			<Select
				classes={{ root: 'flex grow' }}
				class="bg-surface1 shadow-inner! dark:bg-background dark:border-surface3 border border-transparent"
				options={PII_FILTER_OPTIONAL_OPTIONS}
				id={`pii-filter-type-${option.id}-selector`}
				placeholder="Select filter type..."
				searchInDropdown
				onSelect={(selected) => {
					unassignedCustomOptions[i] = { ...unassignedCustomOptions[i], key: selected.id };
					tryCommitUnassignedRow(i);
				}}
			/>
			<Select
				classes={{
					root: 'w-full md:w-82'
				}}
				class="bg-surface1 shadow-inner! dark:bg-background dark:border-surface3 border border-transparent"
				options={PII_FILTER_OPTION_VALUES}
				selected={option.value}
				id={`pii-filter-type-${option.id}`}
				onSelect={(selected) => {
					unassignedCustomOptions[i] = { ...unassignedCustomOptions[i], value: selected.id };
					tryCommitUnassignedRow(i);
				}}
			/>
			<button
				class="icon-button hover:text-red-500"
				onclick={() => {
					unassignedCustomOptions.splice(i, 1);
				}}
			>
				<Trash2 class="size-4" />
			</button>
		</div>
	{/each}
</div>
<div class="flex justify-end">
	<button
		class="button flex items-center gap-1 text-xs"
		onclick={() => {
			unassignedCustomOptions.push({ id: randomUUID(), key: '', value: 'none' });
		}}
	>
		<Plus class="size-4" /> Filter Type
	</button>
</div>

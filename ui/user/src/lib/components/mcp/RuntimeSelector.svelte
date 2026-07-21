<script lang="ts">
	import { CATALOG_SERVER_FIELD_IDS } from '$lib/constants';
	import type { LaunchServerType, Runtime } from '$lib/services/user/types';
	import Select from '../Select.svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		runtime: Runtime;
		serverType: LaunchServerType;
		readonly?: boolean;
		onRuntimeChange?: (runtime: Runtime) => void;
	}
	let { runtime = $bindable(), serverType, readonly = false, onRuntimeChange }: Props = $props();

	// Define available runtime options based on server type
	const runtimeOptions = $derived.by(() => {
		if (serverType === 'remote') {
			return [{ id: 'remote', label: 'Remote' }];
		}

		if (serverType === 'composite') {
			return [{ id: 'composite', label: 'Composite' }];
		}

		return [
			{ id: 'npx', label: 'NPX' },
			{ id: 'uvx', label: 'UVX' },
			{ id: 'containerized', label: 'Containerized' }
		];
	});

	// Automatically set runtime based on server type
	$effect(() => {
		if (serverType === 'remote' && runtime !== 'remote') {
			runtime = 'remote';
			onRuntimeChange?.('remote');
		}

		if (serverType === 'composite' && runtime !== 'composite') {
			runtime = 'composite';
			onRuntimeChange?.('composite');
		}
	});

	// Validate runtime selection
	$effect(() => {
		if (serverType !== 'remote' && runtime === 'remote') {
			// Default to npx if remote is selected for non-remote server
			runtime = 'npx';
			onRuntimeChange?.('npx');
		}

		if (serverType !== 'composite' && runtime === 'composite') {
			runtime = 'composite';
			onRuntimeChange?.('composite');
		}
	});

	function handleRuntimeChange(option: { id: string; label: string }) {
		const newRuntime = option.id as Runtime;
		runtime = newRuntime;
		onRuntimeChange?.(newRuntime);
	}
</script>

<div
	class={twMerge(
		'paper p-4',
		serverType === 'remote' || serverType === 'composite' ? 'hidden' : ''
	)}
	aria-labelledby={`${CATALOG_SERVER_FIELD_IDS.runtime}-heading`}
	id={CATALOG_SERVER_FIELD_IDS.runtime}
>
	<h4 id={`${CATALOG_SERVER_FIELD_IDS.runtime}-heading`} class="text-sm font-semibold">Runtime</h4>

	<div class="flex items-center gap-4">
		<span id="runtime-selector-label" class="text-sm font-light">Type</span>
		<div class="w-full">
			<Select
				id="runtime-selector"
				class="bg-base-200 dark:bg-base-100 dark:border-base-400 flex-1 border border-transparent shadow-none"
				options={runtimeOptions}
				selected={runtime}
				ariaLabelledby="runtime-selector-label"
				ariaDescribedby={!readonly && serverType !== 'remote' ? 'runtime-selector-hint' : undefined}
				onSelect={handleRuntimeChange}
				disabled={readonly || serverType === 'remote'}
			/>
		</div>
	</div>

	{#if !readonly && serverType !== 'remote'}
		<p id="runtime-selector-hint" class="text-muted-content text-xs">
			Choose the runtime environment for your MCP catalog entry.
		</p>
	{/if}
</div>

<script lang="ts">
	import type { Runtime } from '$lib/services/chat/types';
	import Select from '../Select.svelte';

	interface Props {
		runtime: Runtime;
		serverType: 'single' | 'multi' | 'remote' | 'composite';
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
	class="dark:bg-surface1 dark:border-surface3 flex flex-col gap-4 rounded-lg border border-transparent bg-white p-4 shadow-sm {serverType ===
		'remote' || serverType === 'composite'
		? 'hidden'
		: ''}"
>
	<h4 class="text-sm font-semibold">Runtime</h4>

	<div class="flex items-center gap-4">
		<label for="runtime-selector" class="text-sm font-light">Type</label>
		<div class="w-full">
			<Select
				id="runtime-selector"
				class="bg-surface1 dark:bg-surface2 dark:border-surface3 flex-1 border border-transparent shadow-inner"
				options={runtimeOptions}
				selected={runtime}
				onSelect={handleRuntimeChange}
				disabled={readonly || serverType === 'remote'}
			/>
		</div>
	</div>

	{#if !readonly && serverType !== 'remote'}
		<p class="text-xs text-gray-500 dark:text-gray-400">
			Choose the runtime environment for your MCP server.
		</p>
	{/if}
</div>

<script lang="ts">
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import type { DeviceScanPlugin } from '$lib/services/admin/types';
	import { goto } from '$lib/url';
	import { formatBytes, lookupFiles } from '../../_shared/files';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	let scan = $derived(data?.scan);
	let id = $derived(Number(page.params.id));
	let plugin = $derived<DeviceScanPlugin | undefined>(scan?.plugins?.find((p) => p.id === id));
	let backHref = $derived(`/admin/devices/${page.params.device_id}/scans/${page.params.scan_id}`);

	let files = $derived(lookupFiles(scan?.files, plugin?.files));
	let scope = $derived(plugin?.projectPath ? 'project' : 'global');

	let capabilities = $derived(
		plugin
			? [
					{ key: 'rules', has: plugin.hasRules },
					{ key: 'commands', has: plugin.hasCommands },
					{ key: 'hooks', has: plugin.hasHooks }
				].filter((c) => c.has)
			: []
	);

	const duration = PAGE_TRANSITION_DURATION;
</script>

<svelte:head>
	<title>Obot | Plugin {plugin?.name ?? ''}</title>
</svelte:head>

<Layout title={plugin?.name || 'Plugin'} showBackButton onBackButtonClick={() => goto(backHref)}>
	<div
		class="flex flex-col gap-6"
		in:fly={{ x: 100, duration, delay: duration }}
		out:fly={{ x: -100, duration }}
	>
		{#if !scan || !plugin}
			<p class="text-on-surface1 text-sm font-light">Plugin not found in this scan.</p>
		{:else}
			<div class="dark:bg-surface2 bg-background flex flex-col gap-3 rounded-md p-4 shadow-sm">
				<div class="flex flex-wrap items-baseline gap-2">
					<h2 class="font-mono text-xl font-semibold">{plugin.name}</h2>
					{#if plugin.version}
						<span class="text-on-surface1 font-mono text-sm">v{plugin.version}</span>
					{/if}
					<span class="pill-primary bg-primary">{plugin.pluginType}</span>
					<span class="dark:bg-surface3 bg-surface2 rounded px-1.5 py-0.5 font-mono text-xs">
						{plugin.client}
					</span>
					<span class="dark:bg-surface3 bg-surface2 rounded px-1.5 py-0.5 font-mono text-xs">
						{scope}
					</span>
					<span
						class="pill text-xs"
						class:bg-success={plugin.enabled}
						class:bg-surface3={!plugin.enabled}
					>
						{plugin.enabled ? 'enabled' : 'disabled'}
					</span>
				</div>

				<dl class="grid grid-cols-1 gap-x-6 gap-y-2 text-sm md:grid-cols-[max-content_1fr]">
					{#if plugin.description}
						<dt class="text-on-surface1">Description</dt>
						<dd>{plugin.description}</dd>
					{/if}
					{#if plugin.author}
						<dt class="text-on-surface1">Author</dt>
						<dd>{plugin.author}</dd>
					{/if}
					{#if plugin.marketplace}
						<dt class="text-on-surface1">Marketplace</dt>
						<dd class="font-mono text-xs break-all">{plugin.marketplace}</dd>
					{/if}
					{#if plugin.configPath}
						<dt class="text-on-surface1">File</dt>
						<dd class="font-mono text-xs break-all">{plugin.configPath}</dd>
					{/if}
					{#if plugin.projectPath}
						<dt class="text-on-surface1">Project path</dt>
						<dd class="font-mono text-xs break-all">{plugin.projectPath}</dd>
					{/if}
					<dt class="text-on-surface1">Capabilities</dt>
					<dd>
						{#if capabilities.length === 0}
							<span class="text-on-surface1">none detected</span>
						{:else}
							<div class="flex flex-wrap gap-1">
								{#each capabilities as c (c.key)}
									<span
										class="dark:bg-surface3 bg-surface2 rounded px-1.5 py-0.5 font-mono text-xs"
									>
										{c.key}
									</span>
								{/each}
							</div>
						{/if}
					</dd>
				</dl>
			</div>

			<div class="flex flex-col gap-2">
				<h3 class="text-base font-semibold">Supporting files ({files.length})</h3>
				{#if files.length === 0}
					<p class="text-on-surface1 text-sm font-light">No supporting files referenced.</p>
				{:else}
					<div class="flex flex-col gap-3">
						{#each files as { path, file } (path)}
							<div
								class="dark:bg-surface2 bg-background flex flex-col gap-2 rounded-md p-3 shadow-sm"
							>
								<div class="flex flex-wrap items-center gap-2 text-xs">
									<span class="font-mono break-all">{path}</span>
									{#if file}
										<span class="text-on-surface1">{formatBytes(file.sizeBytes)}</span>
										{#if file.oversized}
											<span class="pill bg-warning">oversized</span>
										{/if}
									{:else}
										<span class="text-on-surface1">not collected</span>
									{/if}
								</div>
								{#if file?.content}
									<pre
										class="dark:bg-surface3 bg-surface1 text-on-background max-h-96 overflow-auto rounded p-2 font-mono text-xs">{file.content}</pre>
								{/if}
							</div>
						{/each}
					</div>
				{/if}
			</div>
		{/if}
	</div>
</Layout>

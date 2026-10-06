<script lang="ts">
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { deriveDeviceScope, formatDeviceClient } from '$lib/format.js';
	import { m } from '$lib/i18n';
	import type { DeviceScanPlugin } from '$lib/services/user/types';
	import { goto } from '$lib/url';
	import { formatBytes, lookupFiles } from '../../_shared/files';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	let scan = $derived(data?.scan);
	let id = $derived(Number(page.params.id));
	let plugin = $derived<DeviceScanPlugin | undefined>(scan?.plugins?.find((p) => p.id === id));

	let backHref = $derived(
		`/inventory/devices/${page.params.device_id}/scans/${page.params.scan_id}`
	);

	let files = $derived(lookupFiles(scan?.files, plugin?.files));
	let scope = $derived(deriveDeviceScope(plugin?.projectPath));

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
	<title
		>{m.inventory_enforcement_devices_page_title_plugin_named({ name: plugin?.name ?? '' })}</title
	>
</svelte:head>

<Layout
	title={plugin?.name || m.inventory_enforcement_devices_plugin()}
	showBackButton
	onBackButtonClick={() => goto(backHref)}
>
	<div
		class="flex flex-col gap-6"
		in:fly={{ x: 100, duration, delay: duration }}
		out:fly={{ x: -100, duration }}
	>
		{#if !scan || !plugin}
			<p class="text-muted-content text-sm font-light">
				{m.inventory_enforcement_devices_plugin_not_found_in_scan()}
			</p>
		{:else}
			<div class="dark:bg-base-300 bg-base-100 flex flex-col gap-3 rounded-md p-4 shadow-sm">
				<div class="flex flex-wrap items-baseline gap-2">
					<h2 class="text-xl font-semibold">{plugin.name}</h2>
					{#if plugin.version}
						<span class="text-muted-content text-sm">v{plugin.version}</span>
					{/if}
					<span class="pill-primary bg-primary">{plugin.pluginType}</span>
					<span class="dark:bg-base-400 bg-base-300 rounded px-1.5 py-0.5 text-xs">
						{formatDeviceClient(plugin.client, plugin.projectPath)}
					</span>
					<span class="dark:bg-base-400 bg-base-300 rounded px-1.5 py-0.5 text-xs">
						{scope}
					</span>
					<span
						class="pill text-xs"
						class:bg-success={plugin.enabled}
						class:bg-base-400={!plugin.enabled}
					>
						{plugin.enabled
							? m.inventory_enforcement_devices_enabled()
							: m.inventory_enforcement_devices_disabled()}
					</span>
				</div>

				<dl class="grid grid-cols-1 gap-x-6 gap-y-2 text-sm md:grid-cols-[max-content_1fr]">
					{#if plugin.description}
						<dt class="text-muted-content">{m.core_description()}</dt>
						<dd>{plugin.description}</dd>
					{/if}
					{#if plugin.author}
						<dt class="text-muted-content">{m.inventory_enforcement_devices_label_author()}</dt>
						<dd>{plugin.author}</dd>
					{/if}
					{#if plugin.marketplace}
						<dt class="text-muted-content">
							{m.inventory_enforcement_devices_label_marketplace()}
						</dt>
						<dd class="break-all">{plugin.marketplace}</dd>
					{/if}
					{#if plugin.configPath}
						<dt class="text-muted-content">{m.inventory_enforcement_devices_label_file()}</dt>
						<dd class="break-all">{plugin.configPath}</dd>
					{/if}
					{#if plugin.projectPath}
						<dt class="text-muted-content">
							{m.inventory_enforcement_devices_label_project_path()}
						</dt>
						<dd class="break-all">{plugin.projectPath}</dd>
					{/if}
					<dt class="text-muted-content">{m.inventory_enforcement_devices_col_capabilities()}</dt>
					<dd>
						{#if capabilities.length === 0}
							<span class="text-muted-content"
								>{m.inventory_enforcement_devices_none_detected()}</span
							>
						{:else}
							<div class="flex flex-wrap gap-2">
								{#each capabilities as c (c.key)}
									<span class="dark:bg-base-400 bg-base-300 rounded px-1.5 py-0.5 text-xs">
										{c.key}
									</span>
								{/each}
							</div>
						{/if}
					</dd>
				</dl>
			</div>

			<div class="flex flex-col gap-2">
				<h3 class="text-base font-semibold">
					{m.inventory_enforcement_devices_supporting_files({ count: files.length })}
				</h3>
				{#if files.length === 0}
					<p class="text-muted-content text-sm font-light">
						{m.inventory_enforcement_devices_no_supporting_files()}
					</p>
				{:else}
					<div class="flex flex-col gap-3">
						{#each files as { path, file } (path)}
							<div
								class="dark:bg-base-300 bg-base-100 flex flex-col gap-2 rounded-md p-3 shadow-sm"
							>
								<div class="flex flex-wrap items-center gap-2 text-xs">
									<span class="break-all">{path}</span>
									{#if file}
										<span class="text-muted-content">{formatBytes(file.sizeBytes)}</span>
										{#if file.oversized}
											<span class="pill bg-warning"
												>{m.inventory_enforcement_devices_oversized()}</span
											>
										{/if}
									{:else}
										<span class="text-muted-content"
											>{m.inventory_enforcement_devices_not_collected()}</span
										>
									{/if}
								</div>
								{#if file?.content}
									<pre
										class="dark:bg-base-400 bg-base-200 max-h-96 overflow-auto rounded p-2 font-mono text-xs mb-0 mt-2">{file.content}</pre>
								{/if}
							</div>
						{/each}
					</div>
				{/if}
			</div>
		{/if}
	</div>
</Layout>

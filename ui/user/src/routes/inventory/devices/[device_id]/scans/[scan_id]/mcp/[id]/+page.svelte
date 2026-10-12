<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { deriveDeviceScope, formatDeviceClient } from '$lib/format.js';
	import { m } from '$lib/i18n';
	import type { DeviceScanMCPServer } from '$lib/services/user/types';
	import { goto } from '$lib/url';
	import { findParentPlugin, shortHash } from '../../_shared/files';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	let scan = $derived(data?.scan);
	let id = $derived(Number(page.params.id));
	let server = $derived<DeviceScanMCPServer | undefined>(
		scan?.mcpServers?.find((srv) => srv.id === id)
	);
	let backHref = $derived(
		`/inventory/devices/${page.params.device_id}/scans/${page.params.scan_id}`
	);

	let endpoint = $derived(
		server
			? server.transport === 'stdio'
				? [server.command ?? '', ...(server.args ?? [])].join(' ').trim()
				: (server.url ?? '')
			: ''
	);

	let parentPlugin = $derived(findParentPlugin(scan, server?.file));
	let scope = $derived(deriveDeviceScope(server?.projectPath));

	function renderConfig(s: DeviceScanMCPServer): string {
		const entry: Record<string, unknown> = { type: s.transport };
		if (s.command) entry.command = s.command;
		if (s.args && s.args.length > 0) entry.args = s.args;
		if (s.url) entry.url = s.url;
		if (s.envKeys && s.envKeys.length > 0) {
			entry.env = Object.fromEntries(s.envKeys.map((k) => [k, '<set>']));
		}
		if (s.headerKeys && s.headerKeys.length > 0) {
			entry.headers = Object.fromEntries(s.headerKeys.map((k) => [k, '<set>']));
		}
		return JSON.stringify({ [s.name]: entry }, null, 2);
	}

	const duration = PAGE_TRANSITION_DURATION;
</script>

<svelte:head>
	<title
		>{m.inventory_enforcement_devices_page_title_mcp_server_named({
			name: server?.name ?? ''
		})}</title
	>
</svelte:head>

<Layout
	title={server?.name || m.inventory_enforcement_mcp_server()}
	showBackButton
	onBackButtonClick={() => {
		if (typeof window !== 'undefined' && window.history.length > 1) {
			window.history.back();
		} else {
			goto(backHref);
		}
	}}
>
	<div
		class="flex flex-col gap-6"
		in:fly={{ x: 100, duration, delay: duration }}
		out:fly={{ x: -100, duration }}
	>
		{#if !scan || !server}
			<p class="text-muted-content text-sm font-light">
				{m.inventory_enforcement_devices_mcp_not_found_in_scan()}
			</p>
		{:else}
			<div class="dark:bg-base-300 bg-base-100 flex flex-col gap-3 rounded-md p-4 shadow-sm">
				<div class="flex flex-wrap items-baseline gap-2">
					<h2 class="text-xl font-semibold">{server.name}</h2>
					<span class="pill-primary bg-primary">{server.transport}</span>
					<span class="dark:bg-base-400 bg-base-300 rounded px-1.5 py-0.5 text-xs">
						{formatDeviceClient(server.client, server.projectPath)}
					</span>
					<span class="dark:bg-base-400 bg-base-300 rounded px-1.5 py-0.5 text-xs">
						{scope}
					</span>
				</div>

				<dl class="grid grid-cols-1 gap-x-6 gap-y-2 text-sm md:grid-cols-[max-content_1fr]">
					{#if endpoint}
						<dt class="text-muted-content">{m.inventory_enforcement_col_endpoint()}</dt>
						<dd class="break-all">{endpoint}</dd>
					{/if}
					{#if server.command}
						<dt class="text-muted-content">
							{m.inventory_enforcement_enforcement_events_command()}
						</dt>
						<dd class="font-mono break-all">{server.command}</dd>
					{/if}
					{#if server.args && server.args.length > 0}
						<dt class="text-muted-content">{m.inventory_enforcement_devices_label_args()}</dt>
						<dd class="text-xs break-all">
							{#each server.args as arg, i (i)}
								<span class="dark:bg-base-400 bg-base-300 mr-1 inline-block rounded px-1.5 py-0.5">
									{arg}
								</span>
							{/each}
						</dd>
					{/if}
					{#if server.url}
						<dt class="text-muted-content">{m.core_col_url()}</dt>
						<dd class="break-all">{server.url}</dd>
					{/if}
					<dt class="text-muted-content">{m.inventory_enforcement_label_env_keys()}</dt>
					<dd>
						{#if server.envKeys && server.envKeys.length > 0}
							<div class="flex flex-wrap gap-2">
								{#each server.envKeys as k (k)}
									<span class="dark:bg-base-400 bg-base-300 rounded px-1.5 py-0.5 text-xs">
										{k}
									</span>
								{/each}
							</div>
						{:else}
							<span class="text-muted-content">{m.inventory_enforcement_devices_none()}</span>
						{/if}
					</dd>
					<dt class="text-muted-content">{m.inventory_enforcement_label_header_keys()}</dt>
					<dd>
						{#if server.headerKeys && server.headerKeys.length > 0}
							<div class="flex flex-wrap gap-2">
								{#each server.headerKeys as k (k)}
									<span class="dark:bg-base-400 bg-base-300 rounded px-1.5 py-0.5 text-xs">
										{k}
									</span>
								{/each}
							</div>
						{:else}
							<span class="text-muted-content">{m.inventory_enforcement_devices_none()}</span>
						{/if}
					</dd>
					{#if server.file}
						<dt class="text-muted-content">{m.inventory_enforcement_devices_label_file()}</dt>
						<dd class="text-sm break-all">{server.file}</dd>
					{/if}
					{#if parentPlugin}
						<dt class="text-muted-content">
							{m.inventory_enforcement_devices_label_part_of_plugin()}
						</dt>
						<dd>
							<a
								class="text-sm text-link"
								href={resolve(
									`/inventory/devices/${page.params.device_id}/scans/${page.params.scan_id}/plugins/${parentPlugin.id}`
								)}
							>
								{parentPlugin.name}
							</a>
						</dd>
					{/if}
					{#if server.projectPath}
						<dt class="text-muted-content">
							{m.inventory_enforcement_devices_label_project_path()}
						</dt>
						<dd class="break-all">{server.projectPath}</dd>
					{/if}
					{#if server.configHash}
						<dt class="text-muted-content">
							{m.inventory_enforcement_devices_label_config_hash()}
						</dt>
						<dd class="flex items-center gap-1">
							<span class="text-sm" use:tooltip={server.configHash}>
								{shortHash(server.configHash)}
							</span>
							<CopyButton text={server.configHash} />
						</dd>
					{/if}
				</dl>
			</div>

			<div class="flex flex-col gap-2">
				<div class="flex items-center justify-between">
					<h3 class="text-base font-semibold">{m.inventory_enforcement_configuration_tab()}</h3>
					<CopyButton showTextLeft text={renderConfig(server)} />
				</div>
				<div class="dark:bg-base-300 bg-base-100 flex flex-col gap-2 rounded-md p-3 shadow-sm">
					<pre
						class="dark:bg-base-400 bg-base-200 max-h-96 overflow-auto rounded p-2 font-mono text-xs mb-0 mt-2">{renderConfig(
							server
						)}</pre>
					<p class="text-muted-content text-xs">
						{m.inventory_enforcement_devices_reconstructed_prefix()} <code>&lt;set&gt;</code>
						{m.inventory_enforcement_devices_reconstructed_suffix()}
					</p>
				</div>
			</div>
		{/if}
	</div>
</Layout>

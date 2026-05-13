<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import type { DeviceScanMCPServer } from '$lib/services/admin/types';
	import { goto } from '$lib/url';
	import { findParentPlugin, shortHash } from '../../_shared/files';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	let scan = $derived(data?.scan);
	let id = $derived(Number(page.params.id));
	let server = $derived<DeviceScanMCPServer | undefined>(
		scan?.mcpServers?.find((m) => m.id === id)
	);
	let backHref = $derived(`/admin/devices/${page.params.device_id}/scans/${page.params.scan_id}`);

	let endpoint = $derived(
		server
			? server.transport === 'stdio'
				? [server.command ?? '', ...(server.args ?? [])].join(' ').trim()
				: (server.url ?? '')
			: ''
	);

	let parentPlugin = $derived(findParentPlugin(scan, server?.file));
	let scope = $derived(server?.projectPath ? 'project' : 'global');

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
	<title>Obot | MCP Server {server?.name ?? ''}</title>
</svelte:head>

<Layout
	title={server?.name || 'MCP Server'}
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
			<p class="text-on-surface1 text-sm font-light">MCP server not found in this scan.</p>
		{:else}
			<div class="dark:bg-surface2 bg-background flex flex-col gap-3 rounded-md p-4 shadow-sm">
				<div class="flex flex-wrap items-baseline gap-2">
					<h2 class="font-mono text-xl font-semibold">{server.name}</h2>
					<span class="pill-primary bg-primary">{server.transport}</span>
					<span class="dark:bg-surface3 bg-surface2 rounded px-1.5 py-0.5 font-mono text-xs">
						{server.client}
					</span>
					<span class="dark:bg-surface3 bg-surface2 rounded px-1.5 py-0.5 font-mono text-xs">
						{scope}
					</span>
				</div>

				<dl class="grid grid-cols-1 gap-x-6 gap-y-2 text-sm md:grid-cols-[max-content_1fr]">
					{#if endpoint}
						<dt class="text-on-surface1">Endpoint</dt>
						<dd class="font-mono break-all">{endpoint}</dd>
					{/if}
					{#if server.command}
						<dt class="text-on-surface1">Command</dt>
						<dd class="font-mono break-all">{server.command}</dd>
					{/if}
					{#if server.args && server.args.length > 0}
						<dt class="text-on-surface1">Args</dt>
						<dd class="font-mono text-xs break-all">
							{#each server.args as arg, i (i)}
								<span class="dark:bg-surface3 bg-surface2 mr-1 inline-block rounded px-1.5 py-0.5">
									{arg}
								</span>
							{/each}
						</dd>
					{/if}
					{#if server.url}
						<dt class="text-on-surface1">URL</dt>
						<dd class="font-mono break-all">{server.url}</dd>
					{/if}
					<dt class="text-on-surface1">Env keys</dt>
					<dd>
						{#if server.envKeys && server.envKeys.length > 0}
							<div class="flex flex-wrap gap-1">
								{#each server.envKeys as k (k)}
									<span
										class="dark:bg-surface3 bg-surface2 rounded px-1.5 py-0.5 font-mono text-xs"
									>
										{k}
									</span>
								{/each}
							</div>
						{:else}
							<span class="text-on-surface1">none</span>
						{/if}
					</dd>
					<dt class="text-on-surface1">Header keys</dt>
					<dd>
						{#if server.headerKeys && server.headerKeys.length > 0}
							<div class="flex flex-wrap gap-1">
								{#each server.headerKeys as k (k)}
									<span
										class="dark:bg-surface3 bg-surface2 rounded px-1.5 py-0.5 font-mono text-xs"
									>
										{k}
									</span>
								{/each}
							</div>
						{:else}
							<span class="text-on-surface1">none</span>
						{/if}
					</dd>
					{#if server.file}
						<dt class="text-on-surface1">File</dt>
						<dd class="font-mono text-xs break-all">{server.file}</dd>
					{/if}
					{#if parentPlugin}
						<dt class="text-on-surface1">Part of plugin</dt>
						<dd>
							<a
								class="text-link font-mono"
								href={resolve(
									`/admin/devices/${page.params.device_id}/scans/${page.params.scan_id}/plugins/${parentPlugin.id}`
								)}
							>
								{parentPlugin.name}
							</a>
						</dd>
					{/if}
					{#if server.projectPath}
						<dt class="text-on-surface1">Project path</dt>
						<dd class="font-mono text-xs break-all">{server.projectPath}</dd>
					{/if}
					{#if server.configHash}
						<dt class="text-on-surface1">Config hash</dt>
						<dd class="flex items-center gap-1">
							<span class="font-mono text-xs" use:tooltip={server.configHash}>
								{shortHash(server.configHash)}
							</span>
							<CopyButton text={server.configHash} />
						</dd>
					{/if}
				</dl>
			</div>

			<div class="flex flex-col gap-2">
				<div class="flex items-center justify-between">
					<h3 class="text-base font-semibold">Configuration</h3>
					<CopyButton text={renderConfig(server)} />
				</div>
				<div class="dark:bg-surface2 bg-background flex flex-col gap-2 rounded-md p-3 shadow-sm">
					<pre
						class="dark:bg-surface3 bg-surface1 text-on-background max-h-96 overflow-auto rounded p-2 font-mono text-xs">{renderConfig(
							server
						)}</pre>
					<p class="text-on-surface1 text-xs">
						Reconstructed from parsed fields. <code>&lt;set&gt;</code> indicates the key was present but
						the value was not captured.
					</p>
				</div>
			</div>
		{/if}
	</div>
</Layout>

<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import type { DeviceScanSkill } from '$lib/services/admin/types';
	import { goto } from '$lib/url';
	import { findParentPlugin, formatBytes, lookupFiles } from '../../_shared/files';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	let scan = $derived(data?.scan);
	let id = $derived(Number(page.params.id));
	let skill = $derived<DeviceScanSkill | undefined>(scan?.skills?.find((s) => s.id === id));
	let backHref = $derived(`/admin/devices/${page.params.device_id}/scans/${page.params.scan_id}`);

	let files = $derived(lookupFiles(scan?.files, skill?.files));
	let parentPlugin = $derived(findParentPlugin(scan, skill?.file));
	let scope = $derived(skill?.projectPath ? 'project' : 'global');
	let clientLabel = $derived(skill?.client || '—');

	const duration = PAGE_TRANSITION_DURATION;
</script>

<svelte:head>
	<title>Obot | Skill {skill?.name ?? ''}</title>
</svelte:head>

<Layout
	title={skill?.name || 'Skill'}
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
		{#if !scan || !skill}
			<p class="text-muted-content text-sm font-light">Skill not found in this scan.</p>
		{:else}
			<div class="dark:bg-base-300 bg-base-100 flex flex-col gap-3 rounded-md p-4 shadow-sm">
				<div class="flex flex-wrap items-baseline gap-2">
					<h2 class="font-mono text-xl font-semibold">{skill.name}</h2>
					<span class="dark:bg-base-400 bg-base-300 rounded px-1.5 py-0.5 font-mono text-xs">
						{clientLabel}
					</span>
					<span class="dark:bg-base-400 bg-base-300 rounded px-1.5 py-0.5 font-mono text-xs">
						{scope}
					</span>
					{#if skill.hasScripts}
						<span class="pill-primary bg-primary">scripts</span>
					{/if}
				</div>

				<dl class="grid grid-cols-1 gap-x-6 gap-y-2 text-sm md:grid-cols-[max-content_1fr]">
					{#if skill.description}
						<dt class="text-muted-content">Description</dt>
						<dd>{skill.description}</dd>
					{/if}
					{#if skill.gitRemoteURL}
						<dt class="text-muted-content">Git remote</dt>
						<dd class="font-mono text-xs break-all">{skill.gitRemoteURL}</dd>
					{/if}
					{#if skill.file}
						<dt class="text-muted-content">File</dt>
						<dd class="font-mono text-xs break-all">{skill.file}</dd>
					{/if}
					{#if skill.projectPath}
						<dt class="text-muted-content">Project path</dt>
						<dd class="font-mono text-xs break-all">{skill.projectPath}</dd>
					{/if}
					{#if parentPlugin}
						<dt class="text-muted-content">Part of plugin</dt>
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
				</dl>
			</div>

			<div class="flex flex-col gap-2">
				<h3 class="text-base font-semibold">Supporting files ({files.length})</h3>
				{#if files.length === 0}
					<p class="text-muted-content text-sm font-light">No supporting files referenced.</p>
				{:else}
					<div class="flex flex-col gap-3">
						{#each files as { path, file } (path)}
							<div
								class="dark:bg-base-300 bg-base-100 flex flex-col gap-2 rounded-md p-3 shadow-sm"
							>
								<div class="flex flex-wrap items-center gap-2 text-xs">
									<span class="font-mono break-all">{path}</span>
									{#if file}
										<span class="text-muted-content">{formatBytes(file.sizeBytes)}</span>
										{#if file.oversized}
											<span class="pill bg-warning">oversized</span>
										{/if}
									{:else}
										<span class="text-muted-content">not collected</span>
									{/if}
								</div>
								{#if file?.content}
									<pre
										class="dark:bg-base-400 bg-base-200 text-base-content max-h-96 overflow-auto rounded p-2 font-mono text-xs">{file.content}</pre>
								{/if}
							</div>
						{/each}
					</div>
				{/if}
			</div>
		{/if}
	</div>
</Layout>

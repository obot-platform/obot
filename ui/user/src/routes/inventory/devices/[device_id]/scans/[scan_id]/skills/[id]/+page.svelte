<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { deriveDeviceScope, formatDeviceClient } from '$lib/format.js';
	import { m } from '$lib/i18n';
	import type { DeviceScanSkill } from '$lib/services/user/types';
	import { goto } from '$lib/url';
	import { findParentPlugin, formatBytes, lookupFiles } from '../../_shared/files';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	let scan = $derived(data?.scan);
	let id = $derived(Number(page.params.id));
	let skill = $derived<DeviceScanSkill | undefined>(scan?.skills?.find((s) => s.id === id));
	let backHref = $derived(
		`/inventory/devices/${page.params.device_id}/scans/${page.params.scan_id}`
	);

	let files = $derived(lookupFiles(scan?.files, skill?.files));
	let parentPlugin = $derived(findParentPlugin(scan, skill?.file));
	let scope = $derived(deriveDeviceScope(skill?.projectPath));
	let clientLabel = $derived(formatDeviceClient(skill?.client, skill?.projectPath));

	const duration = PAGE_TRANSITION_DURATION;
</script>

<svelte:head>
	<title
		>{m.inventory_enforcement_devices_page_title_skill_named({ name: skill?.name ?? '' })}</title
	>
</svelte:head>

<Layout
	title={skill?.name || m.inventory_enforcement_skill()}
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
			<p class="text-muted-content text-sm font-light">
				{m.inventory_enforcement_devices_skill_not_found_in_scan()}
			</p>
		{:else}
			<div class="dark:bg-base-300 bg-base-100 flex flex-col gap-3 rounded-md p-4 shadow-sm">
				<div class="flex flex-wrap items-baseline gap-2">
					<h2 class="text-xl font-semibold">{skill.name}</h2>
					<span class="dark:bg-base-400 bg-base-300 rounded px-1.5 py-0.5 text-xs">
						{clientLabel}
					</span>
					<span class="dark:bg-base-400 bg-base-300 rounded px-1.5 py-0.5 text-xs">
						{scope}
					</span>
					{#if skill.hasScripts}
						<span class="pill-primary bg-primary">{m.inventory_enforcement_devices_scripts()}</span>
					{/if}
				</div>

				<dl class="grid grid-cols-1 gap-x-6 gap-y-2 text-sm md:grid-cols-[max-content_1fr]">
					{#if skill.description}
						<dt class="text-muted-content">{m.core_description()}</dt>
						<dd>{skill.description}</dd>
					{/if}
					{#if skill.gitRemoteURL}
						<dt class="text-muted-content">{m.inventory_enforcement_label_git_remote()}</dt>
						<dd class="break-all">{skill.gitRemoteURL}</dd>
					{/if}
					{#if skill.file}
						<dt class="text-muted-content">{m.inventory_enforcement_devices_label_file()}</dt>
						<dd class="break-all">{skill.file}</dd>
					{/if}
					{#if skill.projectPath}
						<dt class="text-muted-content">
							{m.inventory_enforcement_devices_label_project_path()}
						</dt>
						<dd class="break-all">{skill.projectPath}</dd>
					{/if}
					{#if parentPlugin}
						<dt class="text-muted-content">
							{m.inventory_enforcement_devices_label_part_of_plugin()}
						</dt>
						<dd>
							<a
								class="text-link text-sm"
								href={resolve(
									`/inventory/devices/${page.params.device_id}/scans/${page.params.scan_id}/plugins/${parentPlugin.id}`
								)}
							>
								{parentPlugin.name}
							</a>
						</dd>
					{/if}
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
										class="dark:bg-base-400 bg-base-200 max-h-96 overflow-auto rounded p-2 text-xs mb-0 mt-2">{file.content}</pre>
								{/if}
							</div>
						{/each}
					</div>
				{/if}
			</div>
		{/if}
	</div>
</Layout>

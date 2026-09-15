<script lang="ts">
	import {
		formatJsonWithDiffHighlighting,
		generateJsonDiff,
		normalizeManifestsForDiff
	} from '$lib/diff';
	import type { VMCP } from '$lib/services';
	import { vmcpComponentDiffServers, vmcpOutdatedComponents } from '$lib/services/vmcps/utils';
	import { mcpServersAndEntries, responsive } from '$lib/stores';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import { Layers, Server } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	let vmcp = $state<VMCP>();
	let selectedIndex = $state(0);
	let diffDialog = $state<ReturnType<typeof ResponsiveDialog>>();

	let entriesMap = $derived(
		new Map(mcpServersAndEntries.current.entries.map((entry) => [entry.id, entry] as const))
	);
	let outdatedComponents = $derived(vmcp ? vmcpOutdatedComponents(vmcp) : []);
	let selectedComponent = $derived(outdatedComponents[selectedIndex]);
	let diffTargets = $derived.by(() => {
		if (!selectedComponent) {
			return {};
		}
		return vmcpComponentDiffServers(
			selectedComponent,
			entriesMap.get(selectedComponent.mcpServerCatalogEntryID)
		);
	});

	export function open(next: VMCP) {
		vmcp = next;
		selectedIndex = 0;
		diffDialog?.open();
	}

	export function close() {
		diffDialog?.close();
	}
</script>

<ResponsiveDialog
	bind:this={diffDialog}
	class="h-dvh w-full max-w-full md:w-[calc(100vw-2em)]"
	classes={{ content: 'p-0' }}
>
	{#snippet titleContent()}
		{#if vmcp}
			<div class="flex items-center gap-2 md:p-4 md:pb-0">
				<div class="bg-base-200 rounded-sm p-1 dark:bg-base-300">
					{#if vmcp.icon}
						<img src={vmcp.icon} alt={vmcp.displayName} class="size-5" />
					{:else}
						<Layers class="size-5" />
					{/if}
				</div>
				{vmcp.displayName || 'vMCP'} | {vmcp.id}
			</div>
		{/if}
	{/snippet}
	{#if outdatedComponents.length > 1}
		<div class="flex flex-wrap gap-2 border-b border-base-200 px-4 py-3 dark:border-base-400">
			{#each outdatedComponents as component, index (component.id ?? component.mcpServerCatalogEntryID)}
				<button
					type="button"
					class={twMerge('btn btn-sm', selectedIndex === index ? 'btn-primary' : 'btn-ghost')}
					onclick={() => (selectedIndex = index)}
				>
					{component.name || component.catalogEntry.manifest.name}
				</button>
			{/each}
		</div>
	{/if}
	{#if diffTargets.fromServer && diffTargets.toServer}
		{@const normalizedManifests = normalizeManifestsForDiff(
			diffTargets.fromServer.manifest,
			diffTargets.toServer.manifest
		)}
		{@const diffManifest = normalizedManifests[0]}
		{@const newServerManifest = normalizedManifests[1]}
		{#if newServerManifest && diffManifest}
			{@const diff = generateJsonDiff(diffManifest, newServerManifest)}
			{#if selectedComponent}
				<div
					class="flex items-center gap-2 border-b border-base-200 px-4 py-2 dark:border-base-400"
				>
					<div class="bg-base-200 rounded-sm p-1 dark:bg-base-300">
						{#if selectedComponent.catalogEntry.manifest.icon}
							<img
								src={selectedComponent.catalogEntry.manifest.icon}
								alt={selectedComponent.name}
								class="size-5"
							/>
						{:else}
							<Server class="size-5" />
						{/if}
					</div>
					<p class="text-sm font-medium">
						{selectedComponent.name || selectedComponent.catalogEntry.manifest.name}
					</p>
				</div>
			{/if}
			{#if !responsive.isMobile}
				<div class="grid h-full grid-cols-2">
					<div class="h-full">
						<h3 class="text-muted-content mb-2 px-4 text-sm font-semibold">Current Version</h3>
						<div
							class="default-scrollbar-thin dark:border-base-400 dark:bg-base-200 h-full overflow-x-auto border-r border-gray-200 bg-gray-50 p-4"
						>
							<div class="font-mono text-sm whitespace-pre">
								{@html formatJsonWithDiffHighlighting(diffManifest, diff, true)}
							</div>
						</div>
					</div>
					<div class="h-full">
						<h3 class="text-muted-content mb-2 px-4 text-sm font-semibold">New Version</h3>
						<div
							class="default-scrollbar-thin dark:border-base-400 dark:bg-base-200 h-full overflow-x-auto bg-gray-50 p-4"
						>
							<div class="font-mono text-sm whitespace-pre">
								{@html formatJsonWithDiffHighlighting(newServerManifest, diff, false)}
							</div>
						</div>
					</div>
				</div>
			{:else}
				<div class="h-full w-full pl-2">
					<h3 class="text-muted-content mb-2 text-sm font-semibold">Source Diff</h3>
					<div
						class="default-scrollbar-thin dark:bg-base-200 h-full overflow-auto rounded-sm bg-gray-50 pt-4"
					>
						{#each diff.unifiedLines as line, i (i)}
							{@const type = line.startsWith('+')
								? 'added'
								: line.startsWith('-')
									? 'removed'
									: 'unchanged'}
							{@const content = line.startsWith('+') || line.startsWith('-') ? line.slice(1) : line}
							{@const prefix = line.startsWith('+') ? '+' : line.startsWith('-') ? '-' : ' '}
							<div
								class={twMerge(
									'font-mono text-sm whitespace-pre',
									type === 'added'
										? 'bg-success/10 text-success'
										: type === 'removed'
											? 'bg-error/10 text-error'
											: 'text-muted-content'
								)}
							>
								{prefix}{content}
							</div>
						{/each}
					</div>
				</div>
			{/if}
		{:else}
			<div class="flex items-center justify-center py-8">
				<p class="text-muted-content">Unable to compare manifests. Missing manifest data.</p>
			</div>
		{/if}
	{:else}
		<div class="flex items-center justify-center py-8">
			<p class="text-muted-content">Unable to compare manifests. Missing manifest data.</p>
		</div>
	{/if}
</ResponsiveDialog>

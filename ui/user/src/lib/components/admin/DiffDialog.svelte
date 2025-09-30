<script lang="ts">
	import { generateJsonDiff, formatJsonWithDiffHighlighting } from '$lib/diff';
	import { responsive } from '$lib/stores';
	import { Server } from 'lucide-svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import { twMerge } from 'tailwind-merge';
	import type { MCPCatalogEntry, MCPCatalogServer } from '$lib/services';

	interface Props {
		fromServer?: MCPCatalogServer;
		toServer?: MCPCatalogServer | MCPCatalogEntry;
	}

	let { fromServer, toServer }: Props = $props();

	let diffDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	export function open() {
		diffDialog?.open();
	}
	export function close() {
		diffDialog?.close();
	}
</script>

<ResponsiveDialog bind:this={diffDialog} class="h-dvh w-full max-w-full p-0 md:w-[calc(100vw-2em)]">
	{#snippet titleContent()}
		{#if fromServer?.manifest}
			<div class="flex items-center gap-2 md:p-4 md:pb-0">
				<div class="bg-surface1 rounded-sm p-1 dark:bg-gray-600">
					{#if fromServer?.manifest?.icon}
						<img src={fromServer.manifest.icon} alt={fromServer.manifest.name} class="size-5" />
					{:else}
						<Server class="size-5" />
					{/if}
				</div>
				{fromServer.manifest.name} | {fromServer.id}
			</div>
		{/if}
	{/snippet}
	{#if toServer && fromServer}
		{@const newServerManifest = toServer.manifest}
		{@const diffManifest = fromServer?.manifest}
		{#if newServerManifest && diffManifest}
			{@const diff = generateJsonDiff(diffManifest, newServerManifest)}
			{#if !responsive.isMobile}
				<div class="grid h-full grid-cols-2">
					<div class="h-full">
						<h3 class="mb-2 px-4 text-sm font-semibold text-gray-600 dark:text-gray-400">
							Current Version
						</h3>
						<div
							class="default-scrollbar-thin dark:border-surface3 dark:bg-surface1 h-full overflow-x-auto border-r border-gray-200 bg-gray-50 p-4"
						>
							<div class="font-mono text-sm whitespace-pre">
								{@html formatJsonWithDiffHighlighting(diffManifest, diff, true)}
							</div>
						</div>
					</div>
					<div class="h-full">
						<h3 class="mb-2 px-4 text-sm font-semibold text-gray-600 dark:text-gray-400">
							New Version
						</h3>
						<div
							class="default-scrollbar-thin dark:border-surface3 dark:bg-surface1 h-full overflow-x-auto bg-gray-50 p-4"
						>
							<div class="font-mono text-sm whitespace-pre">
								{@html formatJsonWithDiffHighlighting(newServerManifest, diff, false)}
							</div>
						</div>
					</div>
				</div>
			{:else}
				<div class="h-full w-full pl-2">
					<h3 class="mb-2 text-sm font-semibold text-gray-600 dark:text-gray-400">Source Diff</h3>
					<div
						class="default-scrollbar-thin dark:bg-surface1 h-full overflow-auto rounded-sm bg-gray-50 pt-4"
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
										? 'bg-green-500/10 text-green-500 dark:bg-green-900/30'
										: type === 'removed'
											? 'bg-red-500/10 text-red-500'
											: 'text-gray-700 dark:text-gray-300'
								)}
							>
								{prefix}{content}
							</div>
						{/each}
					</div>
				</div>
			{/if}
		{/if}
	{:else}
		<div class="flex items-center justify-center py-8">
			<p class="text-gray-500 dark:text-gray-400">
				Unable to compare manifests. Missing manifest data.
			</p>
		</div>
	{/if}
</ResponsiveDialog>

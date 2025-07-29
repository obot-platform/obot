<script lang="ts">
	import type { MCPCatalogEntry, MCPCatalogServer, Project, ProjectMCP } from '$lib/services';
	import { twMerge } from 'tailwind-merge';
	import McpServerInfo from './McpServerInfo.svelte';
	import McpServerTools from './McpServerTools.svelte';

	interface Props {
		entry?: MCPCatalogEntry | MCPCatalogServer | ProjectMCP;
		catalogId?: string;
		onAuthenticate?: () => void;
		project?: Project;
		view?: 'overview' | 'tools';
	}

	let { entry, catalogId, onAuthenticate, project, view = 'overview' }: Props = $props();
	let selected = $state<string>(view);

	const tabs = [
		{ label: 'Overview', view: 'overview' },
		{ label: 'Tools', view: 'tools' }
	];

	$effect(() => {
		selected = view;
	});
</script>

<div class="gap-4mb-8 flex h-full w-full flex-col">
	<div class="flex grow flex-col gap-2">
		<div class="flex w-full items-center gap-2">
			<div class="flex gap-2 py-1 text-sm font-light">
				{#each tabs as tab (tab.view)}
					<button
						onclick={() => {
							selected = tab.view;
						}}
						class={twMerge(
							'w-48 flex-shrink-0 rounded-md border border-transparent px-4 py-2 text-center transition-colors duration-300',
							selected === tab.view && 'dark:bg-surface1 dark:border-surface3 bg-white shadow-sm',
							selected !== tab.view && 'hover:bg-surface3'
						)}
					>
						{tab.label}
					</button>
				{/each}
			</div>
		</div>

		{#if selected === 'overview' && entry}
			<div class="pb-8">
				<McpServerInfo
					{entry}
					descriptionPlaceholder="Add a description for this MCP server in the Configuration tab"
				/>
			</div>
		{:else if selected === 'tools' && entry}
			<McpServerTools {entry} {catalogId} {onAuthenticate} {project} />
		{/if}
	</div>
</div>

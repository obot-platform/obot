<script lang="ts">
	import { Container, Layers, Search, Bot } from 'lucide-svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import { profile } from '$lib/stores';
	import { Group } from '$lib/services';

	interface Props {
		onSelectServerType: (type: 'single' | 'multi' | 'remote' | 'composite' | 'registry' | 'custom') => void;
		entity?: 'catalog' | 'workspace';
	}

	let selectServerTypeDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let { onSelectServerType, entity = 'catalog' }: Props = $props();

	export function open() {
		selectServerTypeDialog?.open();
	}

	export function close() {
		selectServerTypeDialog?.close();
	}
</script>

<ResponsiveDialog title="Add MCP Server" class="md:w-lg" bind:this={selectServerTypeDialog}>
	<div class="my-4 flex flex-col gap-4">
		<button
			class="dark:bg-surface2 hover:bg-surface1 dark:hover:bg-surface3 dark:border-surface3 border-surface2 group flex cursor-pointer items-center gap-4 rounded-md border bg-white px-2 py-4 text-left transition-colors duration-300"
			onclick={() => onSelectServerType('registry')}
		>
			<Bot
				class="size-12 flex-shrink-0 pl-1 text-gray-500 transition-colors group-hover:text-inherit"
			/>
			<div>
				<p class="mb-1 text-sm font-semibold">Add From Registry</p>
				<span class="block text-xs leading-4 text-gray-400 dark:text-gray-600">
					Select and add existing MCP servers or templates from your registry to make them available
					to users.
				</span>
			</div>
		</button>

		{#if profile.current?.groups.includes(Group.POWERUSER_PLUS)}
			<button
				class="dark:bg-surface2 hover:bg-surface1 dark:hover:bg-surface3 dark:border-surface3 border-surface2 group flex cursor-pointer items-center gap-4 rounded-md border bg-white px-2 py-4 text-left transition-colors duration-300"
				onclick={() => onSelectServerType('custom')}
			>
				<Search
					class="size-12 flex-shrink-0 pl-1 text-gray-500 transition-colors group-hover:text-inherit"
				/>
				<div>
					<p class="mb-1 text-sm font-semibold">Launch Custom Server</p>
					<span class="block text-xs leading-4 text-gray-400 dark:text-gray-600">
						Configure and launch a new MCP server from scratch. You'll define the server
						configuration, settings, and deployment parameters to create a custom hosted instance.
					</span>
				</div>
			</button>
		{/if}
		<button
			class="dark:bg-surface2 hover:bg-surface1 dark:hover:bg-surface3 dark:border-surface3 border-surface2 group flex cursor-pointer items-center gap-4 rounded-md border bg-white px-2 py-4 text-left transition-colors duration-300"
			onclick={() => onSelectServerType('remote')}
		>
			<Container
				class="size-12 flex-shrink-0 pl-1 text-gray-500 transition-colors group-hover:text-inherit"
			/>
			<div>
				<p class="mb-1 text-sm font-semibold">Proxy Remote Server</p>
				<span class="block text-xs leading-4 text-gray-400 dark:text-gray-600">
					Connect to an MCP server that's hosted elsewhere. User connections to the remote server
					will be proxied through the Obot gateway for centralized access control and monitoring.
				</span>
			</div>
		</button>
		{#if entity === 'catalog'}
			<button
				class="dark:bg-surface2 hover:bg-surface1 dark:hover:bg-surface3 dark:border-surface3 border-surface2 group flex cursor-pointer items-center gap-4 rounded-md border bg-white px-2 py-4 text-left transition-colors duration-300"
				onclick={() => onSelectServerType('composite')}
			>
				<Layers
					class="size-12 flex-shrink-0 pl-1 text-gray-500 transition-colors group-hover:text-inherit"
				/>
				<div>
					<p class="mb-1 text-sm font-semibold">Add Composite Server</p>
					<span class="block text-xs leading-4 text-gray-400 dark:text-gray-600">
						This option allows you to combine multiple MCP catalog entries into a single unified
						server. Users will connect via a single URL that aggregates tools and resources from all
						component servers.
					</span>
				</div>
			</button>
		{/if}
	</div>
</ResponsiveDialog>

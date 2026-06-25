<script lang="ts">
	import { Group, type LaunchServerType } from '$lib/services';
	import { profile } from '$lib/stores';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import { Container, Layers, Users } from '@lucide/svelte';

	interface Props {
		onSelectServerType: (type: LaunchServerType) => void;
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

<ResponsiveDialog title="Select Server Type" class="md:w-lg" bind:this={selectServerTypeDialog}>
	<div class="flex flex-col gap-4 p-4 md:p-0">
		<button
			class="dark:bg-base-300 hover:bg-base-200 dark:hover:bg-base-400 dark:border-base-400 border-base-300 group bg-base-100 flex cursor-pointer items-center gap-4 rounded-md border px-2 py-4 text-left transition-colors duration-300"
			onclick={() => onSelectServerType('hosted')}
		>
			<Users
				class="text-muted-content size-12 shrink-0 pl-1 transition-colors group-hover:text-inherit"
			/>
			<div>
				<p class="mb-1 text-sm font-semibold">Hosted Server</p>
				<span class="text-muted-content block text-xs leading-4">
					This option is appropriate for setting up a MCP server hosted under the Obot platform. It
					can be configured for individualized access or shared under multiple users.
				</span>
			</div>
		</button>
		<button
			class="dark:bg-base-300 hover:bg-base-200 dark:hover:bg-base-400 dark:border-base-400 border-base-300 group bg-base-100 flex cursor-pointer items-center gap-4 rounded-md border px-2 py-4 text-left transition-colors duration-300"
			onclick={() => onSelectServerType('remote')}
		>
			<Container
				class="text-muted-content size-12 shrink-0 pl-1 transition-colors group-hover:text-inherit"
			/>
			<div>
				<p class="mb-1 text-sm font-semibold">Remote Server</p>
				<span class="text-muted-content block text-xs leading-4">
					This option is appropriate for allowing users to connect to MCP servers that are already
					elsewhere. When a user selects this server, their connection to the remote MCP server will
					go through the Obot gateway.
				</span>
			</div>
		</button>
		{#if entity === 'catalog' && profile.current?.groups.includes(Group.ADMIN)}
			<button
				class="dark:bg-base-300 hover:bg-base-200 dark:hover:bg-base-400 dark:border-base-400 border-base-300 group bg-base-100 flex cursor-pointer items-center gap-4 rounded-md border px-2 py-4 text-left transition-colors duration-300"
				onclick={() => onSelectServerType('composite')}
			>
				<Layers
					class="text-muted-content size-12 shrink-0 pl-1 transition-colors group-hover:text-inherit"
				/>
				<div>
					<p class="mb-1 text-sm font-semibold">Composite Server</p>
					<span class="text-muted-content block text-xs leading-4">
						This option allows you to combine multiple MCP catalog entries into a single unified
						deployment. Users will connect via a single URL that aggregates tools and resources from
						all component entries.
					</span>
				</div>
			</button>
		{/if}
	</div>
</ResponsiveDialog>

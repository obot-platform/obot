<script lang="ts">
	import { Container, User, Users, Layers } from 'lucide-svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import { profile } from '$lib/stores';
	import { Group } from '$lib/services';

	interface Props {
		onSelectServerType: (type: 'single' | 'multi' | 'remote' | 'composite') => void;
	}

	let selectServerTypeDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let { onSelectServerType }: Props = $props();

	export function open() {
		selectServerTypeDialog?.open();
	}

	export function close() {
		selectServerTypeDialog?.close();
	}
</script>

<ResponsiveDialog title="Select Server Type" class="md:w-lg" bind:this={selectServerTypeDialog}>
	<div class="my-4 flex flex-col gap-4">
		<button
			class="dark:bg-surface2 hover:bg-surface1 dark:hover:bg-surface3 dark:border-surface3 border-surface2 group flex cursor-pointer items-center gap-4 rounded-md border bg-white px-2 py-4 text-left transition-colors duration-300"
			onclick={() => onSelectServerType('single')}
		>
			<User
				class="size-12 flex-shrink-0 pl-1 text-gray-500 transition-colors group-hover:text-inherit"
			/>
			<div>
				<p class="mb-1 text-sm font-semibold">Single User Server</p>
				<span class="block text-xs leading-4 text-gray-400 dark:text-gray-600">
					This option is appropriate for servers that require individualized configuration or were
					not designed for multi-user access, such as most studio MCP servers. When a user selects
					this server, a private instance will be created for them.
				</span>
			</div>
		</button>
		{#if profile.current?.groups.includes(Group.POWERUSER_PLUS)}
			<button
				class="dark:bg-surface2 hover:bg-surface1 dark:hover:bg-surface3 dark:border-surface3 border-surface2 group flex cursor-pointer items-center gap-4 rounded-md border bg-white px-2 py-4 text-left transition-colors duration-300"
				onclick={() => onSelectServerType('multi')}
			>
				<Users
					class="size-12 flex-shrink-0 pl-1 text-gray-500 transition-colors group-hover:text-inherit"
				/>
				<div>
					<p class="mb-1 text-sm font-semibold">Multi-User Server</p>
					<span class="block text-xs leading-4 text-gray-400 dark:text-gray-600">
						This option is appropriate for servers designed to handle multiple user connections,
						such as most Streamable HTTP servers. When you create this server, a running instance
						will be deployed and any user with access to this catalog will be able to connect to it.
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
				<p class="mb-1 text-sm font-semibold">Remote Server</p>
				<span class="block text-xs leading-4 text-gray-400 dark:text-gray-600">
					This option is appropriate for allowing users to connect to MCP servers that are already
					elsewhere. When a user selects this server, their connection to the remote MCP server will
					go through the Obot gateway.
				</span>
			</div>
		</button>
		<button
			class="dark:bg-surface2 hover:bg-surface1 dark:hover:bg-surface3 dark:border-surface3 border-surface2 group flex cursor-pointer items-center gap-4 rounded-md border bg-white px-2 py-4 text-left transition-colors duration-300"
			onclick={() => onSelectServerType('composite')}
		>
			<Layers
				class="size-12 flex-shrink-0 pl-1 text-gray-500 transition-colors group-hover:text-inherit"
			/>
			<div>
				<p class="mb-1 text-sm font-semibold">Composite MCP Server</p>
				<span class="block text-xs leading-4 text-gray-400 dark:text-gray-600">
					This option allows you to combine multiple MCP catalog entries into a single unified
					server. Users will connect via a single URL that aggregates tools and resources from all
					component servers.
				</span>
			</div>
		</button>
	</div>
</ResponsiveDialog>

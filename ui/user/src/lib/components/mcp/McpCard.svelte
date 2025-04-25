<script lang="ts">
	import { type MCP } from '$lib/services';
	import { ChevronsRight } from 'lucide-svelte';
	import McpConfig from './McpConfig.svelte';

	interface Props {
		mcp: MCP;
		onSubmit: () => void;
	}
	let { mcp, onSubmit }: Props = $props();
	let dialog = $state<ReturnType<typeof McpConfig>>();
</script>

<button
	onclick={() => dialog?.open()}
	class="card group from-surface2 to-surface1 relative z-20 flex-col overflow-hidden bg-radial-[at_25%_25%] to-75% shadow-md"
>
	<div class="flex h-fit w-full flex-col gap-2 p-4 md:h-auto md:grow">
		<div class="flex w-full">
			<img alt="obot logo" src={mcp.server.icon} class="size-6" />
			<div class="flex grow flex-col gap-2 pl-3 text-left">
				<h4 class="text-sm font-semibold">
					{mcp.server.name}
				</h4>
				<p class="line-clamp-3 grow text-xs font-light text-gray-500">
					{mcp.server.description}
				</p>
			</div>
		</div>
		<div class="flex grow"></div>
		<div
			class="button-secondary border-surface3 group-hover:bg-surface3 flex w-full items-center justify-center gap-1 border text-xs text-gray-500 transition-colors duration-300 group-hover:text-inherit dark:border-black dark:group-hover:bg-black"
		>
			Launch <ChevronsRight class="size-3" />
		</div>
	</div>
</button>

<McpConfig {mcp} {onSubmit} bind:this={dialog} readonly />

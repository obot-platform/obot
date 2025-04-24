<script lang="ts">
	import type { MCP } from '$lib/services';
	import { twMerge } from 'tailwind-merge';
	import { ChevronsRight } from 'lucide-svelte';
	import McpConfig from './McpConfig.svelte';

	interface Props {
		mcp: MCP;
		onSubmit: () => void;
	}

	let dialog = $state<ReturnType<typeof McpConfig>>();
	const { mcp, onSubmit }: Props = $props();
</script>

<button
	onclick={() => dialog?.open()}
	class="card featured-card group relative bg-transparent hover:shadow-none"
>
	<div
		class={twMerge(
			'from-surface2 to-surface1 z-10 flex h-full w-full grow flex-col items-center gap-2 rounded-t-xl bg-radial-[at_25%_25%] to-75% p-4 shadow-md'
		)}
	>
		<img alt="obot logo" src={mcp.server.icon} class="mb-2 flex size-18 flex-shrink-0" />
		<h4 class="line-clamp-2 text-left text-base leading-5.5 font-semibold md:text-lg">
			{mcp.server.name}
		</h4>
		<p class="line-clamp-3 flex text-left text-xs leading-4.5 font-light text-gray-500 md:text-sm">
			{mcp.server.description}
		</p>
		<div class="flex grow"></div>
		<div
			class="button-secondary border-surface3 group-hover:bg-surface3 flex w-full items-center justify-center gap-1 border text-sm text-gray-500 transition-colors duration-300 group-hover:text-inherit dark:border-black dark:group-hover:bg-black"
		>
			Launch <ChevronsRight class="size-4" />
		</div>
	</div>
</button>

<McpConfig {mcp} {onSubmit} bind:this={dialog} readonly />

<style lang="postcss">
	.featured-card {
		&:after {
			content: '';
			z-index: 0;
			position: absolute;
			height: 100%;
			width: 100%;
			bottom: -4px;
			left: 0;
			transition: transform 0.2s ease-in-out;
			background-image: linear-gradient(
				to bottom right,
				var(--color-blue-400),
				var(--color-blue-600)
			);
			border-radius: var(--radius-lg);
		}
	}
</style>

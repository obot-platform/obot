<script lang="ts">
	import { type MCP } from '$lib/services';
	import { twMerge } from 'tailwind-merge';
	import McpConfig from './McpConfig.svelte';
	import { CircleCheckBig } from 'lucide-svelte';

	interface Props {
		mcp: MCP;
		disableOutsideClick?: boolean;
		hideCloseButton?: boolean;
		onSubmit: () => void;
		readonly?: boolean;
		selected?: boolean;
		submitText?: string;
	}
	let {
		mcp,
		disableOutsideClick,
		hideCloseButton,
		onSubmit,
		readonly,
		selected,
		submitText
	}: Props = $props();
	let dialog = $state<ReturnType<typeof McpConfig>>();
</script>

<div class="relative h-full w-full">
	{#if selected}
		<CircleCheckBig class="absolute top-4 right-4 z-25 size-5 text-blue-500" />
	{/if}
	<button
		onclick={() => dialog?.open()}
		class={twMerge(
			'card group from-surface2 to-surface1 relative z-20 h-full w-full flex-col overflow-hidden bg-radial-[at_25%_25%] to-75% shadow-md',
			selected && 'transform-none opacity-50'
		)}
	>
		<div class="flex h-fit w-full flex-col gap-2 p-4 md:h-auto md:grow">
			<div class="flex w-full items-center gap-2">
				<div class="rounded-md bg-gray-50 p-1 dark:bg-gray-600">
					<img alt="obot logo" src={mcp.server.icon} class="size-6" />
				</div>
				<h4 class="text-sm font-semibold">
					{mcp.server.name}
				</h4>
			</div>
			<p class="line-clamp-3 grow text-left text-xs font-light text-gray-500">
				{mcp.server.description}
			</p>
		</div>
	</button>
</div>

<McpConfig
	bind:this={dialog}
	{mcp}
	{disableOutsideClick}
	{hideCloseButton}
	{onSubmit}
	{readonly}
	{selected}
	{submitText}
/>

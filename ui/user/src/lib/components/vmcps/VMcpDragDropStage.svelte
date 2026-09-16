<script lang="ts">
	import { GripVertical, Layers, MousePointer2, Plus, Server } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		class?: string;
	}

	let { class: klass }: Props = $props();
</script>

<div
	class={twMerge(
		'border-base-300 dark:border-base-400 bg-base-200/40 dark:bg-base-200/20 relative h-44 overflow-hidden rounded-md border border-dashed',
		klass
	)}
	aria-hidden="true"
>
	<div class="vmcp-drag-stage-grid text-base-content absolute inset-0 opacity-15"></div>

	<div
		class="vmcp-drag-stage-drop absolute top-3 left-3 flex w-44 flex-col items-center gap-1.5 rounded-lg border px-2 pt-2 pb-2.5"
	>
		<Layers class="text-primary/70 size-3.5" />
		<p class="flex items-center gap-0.5 font-mono text-[0.5rem] tracking-[0.08em] uppercase">
			<Plus class="size-2 shrink-0" /> Create New vMCP
		</p>
		<span class="bg-base-content/15 block h-1 w-20 rounded-full"></span>
	</div>

	<div class="vmcp-drag-stage-travel absolute right-3 bottom-3 w-28">
		<div
			class="bg-base-100 dark:bg-base-300 border-base-300 dark:border-base-400 flex items-center gap-1 rounded-lg border py-1.5 pr-1.5 pl-0.5 shadow-md"
		>
			<GripVertical class="text-muted-content size-2.5 shrink-0" />
			<div class="bg-primary/10 text-primary shrink-0 rounded-md p-1">
				<Server class="size-3" />
			</div>
			<div class="flex min-w-0 grow flex-col gap-1">
				<p class="text-[0.5rem] leading-none font-medium">MCP Server</p>
				<span class="bg-base-content/15 block h-1 w-4/5 rounded-full"></span>
			</div>
		</div>
		<MousePointer2
			class="fill-base-content text-base-100 dark:text-base-300 absolute -right-1 -bottom-2 size-4 stroke-[1.5]"
		/>
	</div>
</div>

<style>
	.vmcp-drag-stage-grid {
		background-image: radial-gradient(currentColor 0.5px, transparent 0.5px);
		background-size: 12px 12px;
	}

	.vmcp-drag-stage-drop {
		--hint-quiet: var(--color-base-300);
		border-color: var(--hint-quiet);
		background-color: color-mix(in oklab, var(--color-base-100) 70%, transparent);
		animation: vmcp-drag-stage-receive 4s cubic-bezier(0.16, 1, 0.3, 1) infinite;
	}

	:global(.dark) .vmcp-drag-stage-drop {
		--hint-quiet: var(--color-base-400);
		background-color: color-mix(in oklab, var(--color-base-300) 70%, transparent);
	}

	.vmcp-drag-stage-travel {
		--hint-dx: -88px;
		--hint-dy: -64px;
		animation: vmcp-drag-stage-travel 4s cubic-bezier(0.65, 0, 0.35, 1) infinite;
	}

	@keyframes vmcp-drag-stage-travel {
		0% {
			opacity: 0;
			transform: translate(0, 0) scale(1);
		}
		8% {
			opacity: 1;
			transform: translate(0, 0) scale(1);
		}
		48%,
		62% {
			opacity: 1;
			transform: translate(var(--hint-dx), var(--hint-dy)) scale(0.94);
		}
		74%,
		100% {
			opacity: 0;
			transform: translate(var(--hint-dx), var(--hint-dy)) scale(0.9);
		}
	}

	@keyframes vmcp-drag-stage-receive {
		0%,
		42% {
			border-color: var(--hint-quiet);
			box-shadow: none;
		}
		52%,
		66% {
			border-color: var(--color-primary);
			box-shadow: 0 0 22px color-mix(in oklab, var(--color-primary) 28%, transparent);
		}
		80%,
		100% {
			border-color: var(--hint-quiet);
			box-shadow: none;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.vmcp-drag-stage-drop,
		.vmcp-drag-stage-travel {
			animation: none;
		}
	}
</style>

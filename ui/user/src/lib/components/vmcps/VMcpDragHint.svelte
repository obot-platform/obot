<script lang="ts">
	import { GripVertical, Layers, MousePointer2, Plus, Server, X } from '@lucide/svelte';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		dragActive?: boolean;
		storageKey?: string;
		class?: string;
	}

	let {
		dragActive = false,
		storageKey = '@obot/seen-vmcp-drag-hint',
		class: klass
	}: Props = $props();

	let dismissed = $state(true);

	onMount(() => {
		dismissed = Boolean(localStorage.getItem(storageKey));
	});

	function dismiss() {
		dismissed = true;
		localStorage.setItem(storageKey, new Date().toISOString());
	}

	$effect(() => {
		if (dragActive) dismiss();
	});
</script>

{#if !dismissed}
	<div class={twMerge('w-60', klass)} in:fly={{ x: 12, duration: 220 }}>
		<div
			class="bg-base-100/90 dark:bg-base-300/90 border-base-300 dark:border-base-400 relative rounded-lg border p-3 shadow-lg backdrop-blur-sm"
		>
			<div
				class="bg-base-100 dark:bg-base-300 border-base-300 dark:border-base-400 absolute top-1/2 -right-1 size-2 -translate-y-1/2 rotate-45 border-t border-r"
				aria-hidden="true"
			></div>

			<div class="flex items-start justify-between gap-2">
				<p class="text-muted-content font-mono text-[0.625rem] tracking-[0.14em] uppercase">
					Drag &amp; Drop
				</p>
				<button
					type="button"
					class="text-muted-content hover:text-base-content -mt-1 -mr-1 rounded-sm p-1 transition-colors"
					aria-label="Dismiss drag and drop tip"
					onclick={dismiss}
				>
					<X class="size-3" />
				</button>
			</div>

			{@render stage()}

			<p class="text-muted-content mt-2 text-xs font-light">
				Drag a server from the panel anywhere onto the canvas to build a vMCP.
			</p>
		</div>
	</div>
{/if}

{#snippet stage()}
	<div
		class="border-base-300 dark:border-base-400 bg-base-200/40 dark:bg-base-200/20 relative mt-2 h-36 overflow-hidden rounded-md border border-dashed"
		aria-hidden="true"
	>
		<div class="vmcp-hint-grid text-base-content absolute inset-0 opacity-15"></div>

		<div
			class="vmcp-hint-drop absolute top-2 left-2 flex w-40 flex-col items-center gap-1.5 rounded-lg border px-2 pt-2 pb-2.5"
		>
			<Layers class="text-primary/70 size-3.5" />
			<p
				class="text-muted-content flex items-center gap-0.5 font-mono text-[0.5rem] tracking-[0.08em] uppercase"
			>
				<Plus class="size-2 shrink-0" /> Create New vMCP
			</p>
			<span class="bg-base-content/15 block h-1 w-20 rounded-full"></span>
		</div>

		<svg class="text-primary/50 absolute inset-0 size-full" viewBox="0 0 216 144" fill="none">
			<circle class="vmcp-hint-anchor" cx="112" cy="122" r="2.5" />
			<path class="vmcp-hint-arrow" d="M112 122C84 124 76 96 104 76" />
			<path class="vmcp-hint-arrowhead" d="M95.2 77.6L104 76L99.6 83.8" />
		</svg>

		<div class="vmcp-hint-travel absolute right-2 bottom-2 w-24">
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
{/snippet}

<style>
	.vmcp-hint-grid {
		background-image: radial-gradient(currentColor 0.5px, transparent 0.5px);
		background-size: 12px 12px;
	}

	.vmcp-hint-drop {
		--hint-quiet: var(--color-base-300);
		border-color: var(--hint-quiet);
		background-color: color-mix(in oklab, var(--color-base-100) 70%, transparent);
		animation: vmcp-hint-receive 4s cubic-bezier(0.16, 1, 0.3, 1) infinite;
	}

	:global(.dark) .vmcp-hint-drop {
		--hint-quiet: var(--color-base-400);
		background-color: color-mix(in oklab, var(--color-base-300) 70%, transparent);
	}

	.vmcp-hint-travel {
		--hint-dx: -72px;
		--hint-dy: -50px;
		animation: vmcp-hint-travel 4s cubic-bezier(0.65, 0, 0.35, 1) infinite;
	}

	.vmcp-hint-anchor {
		fill: currentColor;
	}

	.vmcp-hint-arrow,
	.vmcp-hint-arrowhead {
		stroke: currentColor;
		stroke-width: 1.5;
		stroke-linecap: round;
		stroke-linejoin: round;
	}

	.vmcp-hint-arrow {
		stroke-dasharray: 4 5;
		animation: vmcp-hint-flow 0.6s linear infinite;
	}

	@keyframes vmcp-hint-travel {
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

	@keyframes vmcp-hint-receive {
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

	@keyframes vmcp-hint-flow {
		to {
			stroke-dashoffset: -9px;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.vmcp-hint-drop,
		.vmcp-hint-travel,
		.vmcp-hint-arrow {
			animation: none;
		}
	}
</style>

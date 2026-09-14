<script lang="ts">
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import { GripVertical, Layers, MousePointer2, Plus, Server } from '@lucide/svelte';
	import { onMount } from 'svelte';

	const LEGACY_STORAGE_KEY = '@obot/seen-vmcp-drag-hint';

	interface Props {
		dragActive?: boolean;
		show?: boolean;
		storageKey?: string;
	}

	let {
		dragActive = false,
		show = true,
		storageKey = '@obot/seen-vmcp-introduction'
	}: Props = $props();

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let dismissed = $state(true);

	function hasSeenIntroduction(key: string) {
		return Boolean(localStorage.getItem(key) || localStorage.getItem(LEGACY_STORAGE_KEY));
	}

	onMount(() => {
		dismissed = hasSeenIntroduction(storageKey);
	});

	function dismiss() {
		if (dismissed) return;
		dismissed = true;
		localStorage.setItem(storageKey, new Date().toISOString());
		dialog?.close();
	}

	$effect(() => {
		if (show && !dismissed) {
			dialog?.open();
			return;
		}

		dialog?.close();
	});

	$effect(() => {
		if (dragActive) dismiss();
	});
</script>

<ResponsiveDialog
	bind:this={dialog}
	class="md:max-w-3xl"
	classes={{
		content: 'p-6'
	}}
	hideClose
	disableClickOutside
	onClose={dismiss}
>
	<div class="grid md:grid-cols-2 md:items-center">
		<div class="flex flex-col gap-4">
			<h2 id="vmcp-introduction-title" class="text-2xl font-semibold">What is a vMCP?</h2>
			<p id="vmcp-introduction-description" class="text-muted-content text-sm leading-relaxed">
				A virtual MCP (vMCP) exposes one or more MCP servers through one Obot Gateway endpoint. Each
				component keeps its own deployment and configuration behavior, while the vMCP provides one
				place to manage the connection, tools, and access.
			</p>
			<button type="button" class="btn btn-primary w-full" onclick={dismiss}>Get started</button>
		</div>

		<div
			aria-labelledby="vmcp-introduction-animation-label"
			class="border-l-2 border-primary pl-8 ml-8"
		>
			<h3 class="text-lg font-semibold mb-1">Create your first vMCP!</h3>
			<p
				id="vmcp-introduction-animation-label"
				class="text-muted-content font-mono text-[0.625rem] tracking-[0.14em] uppercase"
			>
				Drag &amp; Drop
			</p>
			{@render stage()}
			<p class="text-muted-content mt-2 text-xs font-light">
				Drag a server from the panel anywhere onto the canvas to begin building a vMCP.
			</p>
		</div>
	</div>
</ResponsiveDialog>

{#snippet stage()}
	<div
		class="border-base-300 dark:border-base-400 bg-base-200/40 dark:bg-base-200/20 relative mt-2 h-44 overflow-hidden rounded-md border border-dashed"
		aria-hidden="true"
	>
		<div class="vmcp-intro-grid text-base-content absolute inset-0 opacity-15"></div>

		<div
			class="vmcp-intro-drop absolute top-3 left-3 flex w-44 flex-col items-center gap-1.5 rounded-lg border px-2 pt-2 pb-2.5"
		>
			<Layers class="text-primary/70 size-3.5" />
			<p
				class="text-muted-content flex items-center gap-0.5 font-mono text-[0.5rem] tracking-[0.08em] uppercase"
			>
				<Plus class="size-2 shrink-0" /> Create New vMCP
			</p>
			<span class="bg-base-content/15 block h-1 w-20 rounded-full"></span>
		</div>

		<div class="vmcp-intro-travel absolute right-3 bottom-3 w-28">
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
	.vmcp-intro-grid {
		background-image: radial-gradient(currentColor 0.5px, transparent 0.5px);
		background-size: 12px 12px;
	}

	.vmcp-intro-drop {
		--hint-quiet: var(--color-base-300);
		border-color: var(--hint-quiet);
		background-color: color-mix(in oklab, var(--color-base-100) 70%, transparent);
		animation: vmcp-intro-receive 4s cubic-bezier(0.16, 1, 0.3, 1) infinite;
	}

	:global(.dark) .vmcp-intro-drop {
		--hint-quiet: var(--color-base-400);
		background-color: color-mix(in oklab, var(--color-base-300) 70%, transparent);
	}

	.vmcp-intro-travel {
		--hint-dx: -88px;
		--hint-dy: -64px;
		animation: vmcp-intro-travel 4s cubic-bezier(0.65, 0, 0.35, 1) infinite;
	}

	@keyframes vmcp-intro-travel {
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

	@keyframes vmcp-intro-receive {
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
		.vmcp-intro-drop,
		.vmcp-intro-travel {
			animation: none;
		}
	}
</style>

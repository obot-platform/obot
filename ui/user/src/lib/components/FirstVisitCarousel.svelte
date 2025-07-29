<script lang="ts">
	import { ChevronLeft, ChevronRight, X } from 'lucide-svelte';
	import ResponsiveDialog from './ResponsiveDialog.svelte';
	import { twMerge } from 'tailwind-merge';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let currentIndex = $state(0);
	let carouselItems = $state<
		{
			title: string;
			description: string;
			imageURL: string;
		}[]
	>([
		{
			title: 'Welcome to Obot',
			imageURL: '/user/images/carousel_welcome.png',
			description: 'Obot is a platform for creating and managing your projects.'
		},
		{
			title: 'Connect to MCP Servers',
			description: 'Use the gateway to connect to authorized MCP servers.',
			imageURL: '/user/images/carousel_mcp.png'
		},
		{
			title: 'Create Projects w/ AI Assistance',
			description:
				'Utilize RAG, memory, and MCP servers to accomplish workflows designed for your needs.',
			imageURL: '/user/images/carousel_chat.png'
		}
	]);
	let totalItems = $derived(carouselItems.length);

	onMount(() => {
		const isRoot = window.location.pathname === '/';
		const hasSeen = localStorage.getItem('hasSeenCarousel') === 'true';
		if (!hasSeen && !isRoot) {
			dialog?.open();
		}
	});

	function close() {
		localStorage.setItem('hasSeenCarousel', 'true');
		dialog?.close();
	}
</script>

<ResponsiveDialog
	bind:this={dialog}
	onClose={() => {
		localStorage.setItem('hasSeenCarousel', 'true');
	}}
	classes={{ header: 'p-0' }}
	class="dark:bg-surface1 w-full bg-gray-50 p-0 pt-6 md:max-w-[900px]"
	hideHeader
>
	{#snippet titleContent()}
		<!-- nothing -->
	{/snippet}
	<div class="relative flex h-full flex-col">
		<button class="icon-button absolute -top-2 right-6 z-10" onclick={close}>
			<X class="size-6" />
		</button>
		<div class="relative flex h-full overflow-hidden pb-0 md:h-[75vh]">
			{#each carouselItems as item, index (index)}
				{#if index === currentIndex}
					<div
						class="absolute inset-0 flex h-full flex-col gap-4"
						in:fly={{ x: 100, delay: 200, duration: 200 }}
						out:fly={{ x: -100, duration: 200 }}
					>
						<h3 class="-translate-y-0.5 text-center text-2xl font-semibold">{item.title}</h3>
						<img
							src={item.imageURL}
							alt={item.title}
							class="h-full w-full rounded-lg object-cover md:h-auto md:max-h-[calc(100%-6rem)] md:object-contain"
						/>
						<p class="text-md text-center text-gray-500">{item.description}</p>
					</div>
				{/if}
			{/each}
		</div>
		<div class="mt-auto flex flex-col gap-2">
			<div class="flex w-full justify-between gap-4 px-6">
				<button
					class="icon-button"
					onclick={() => {
						currentIndex = Math.max(0, currentIndex - 1);
					}}
				>
					<ChevronLeft class="size-6" />
				</button>
				<div class="flex items-center justify-center gap-1">
					{#each carouselItems as _item, index (index)}
						<div
							class={twMerge(
								'h-1 w-3 bg-gray-600 transition-colors duration-200',
								currentIndex === index && 'bg-blue-500'
							)}
						></div>
					{/each}
				</div>
				<button
					class="icon-button"
					onclick={() => {
						currentIndex = Math.min(totalItems - 1, currentIndex + 1);
					}}
				>
					<ChevronRight class="size-6" />
				</button>
			</div>
			<div class="bg-surface2 flex w-full justify-end px-6 py-2">
				<button class="button self-end" onclick={close}>Close</button>
			</div>
		</div>
	</div>
</ResponsiveDialog>

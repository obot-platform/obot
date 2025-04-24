<script lang="ts">
	import { Pencil, X } from 'lucide-svelte';
	import { getLayout } from '$lib/context/layout.svelte';
	import { responsive } from '$lib/stores';
	import { fade, fly, slide } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	const layout = getLayout();

	let hover = $state(false);
</script>

<button
	data-testid="obot-editor-btn"
	onmouseenter={() => (hover = true)}
	onmouseleave={() => (hover = false)}
	onclick={() => {
		if (layout.projectEditorOpen) {
			layout.projectEditorOpen = false;
			return;
		}

		layout.projectEditorOpen = true;
		layout.sidebarOpen = false;
	}}
	class={twMerge(
		'group text-gray relative mr-1 flex items-center rounded-full border p-2 text-xs transition-[background-color] duration-200',
		layout.projectEditorOpen
			? 'border-blue bg-blue text-white md:px-4'
			: 'border-surface3 hover:bg-blue bg-transparent hover:px-4 hover:text-white active:bg-blue-700'
	)}
	transition:fade
>
	{#if layout.projectEditorOpen}
		<X class="h-5 w-5" />
	{:else}
		<Pencil class="h-5 w-5" />
	{/if}
	{#if layout.projectEditorOpen && !responsive.isMobile}
		<span class="ml-1">Exit Editor</span>
	{:else if hover && !responsive.isMobile}
		<span class="flex h-5 items-center" transition:slide={{ axis: 'x' }}>
			<span class="ms-2 inline-block text-nowrap delay-250" transition:fly={{ x: 50 }}>
				Obot Editor
			</span>
		</span>
	{/if}
</button>

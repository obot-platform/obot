<script lang="ts">
	import { getLayout } from '$lib/context/layout.svelte';
	import { type Project } from '$lib/services';
	import { X } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		navBar?: boolean;
		project: Project;
		class?: string;
		currentThreadID?: string;
	}

	let { navBar = false, class: className }: Props = $props();

	const layout = getLayout();
	let show = $derived(navBar || layout.items.length <= 1);
</script>

{#if show}
	<div class={twMerge('flex items-start', className)}>
		<button
			class="icon-button"
			onclick={() => {
				layout.fileEditorOpen = false;
			}}
		>
			<X class="h-5 w-5" />
		</button>
	</div>
{/if}

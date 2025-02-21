<script lang="ts">
	import { Share as ShareIcon } from 'lucide-svelte';
	import { getLayout } from '$lib/context/layout.svelte';
	import { fade } from 'svelte/transition';
	import Share from '$lib/components/edit/Share.svelte';
	import type { Project } from '$lib/services';
	import { X } from 'lucide-svelte/icons';

	interface Props {
		project: Project;
	}

	let { project }: Props = $props();
	let layout = getLayout();
	let shareDialog = $state<HTMLDialogElement>();
</script>

{#if !layout.projectEditorOpen}
	<button class="icon-button" transition:fade onclick={() => shareDialog?.showModal()}>
		<ShareIcon class="icon-default" />
	</button>
	<dialog class="colors-surface1 min-w-[320px] rounded-3xl p-5" bind:this={shareDialog}>
		<h2 class="font-semibold">Sharing</h2>
		<Share dialog {project} />
		<button class="absolute right-5 top-5" onclick={() => shareDialog?.close()}>
			<X class="icon-default" />
		</button>
	</dialog>
{/if}

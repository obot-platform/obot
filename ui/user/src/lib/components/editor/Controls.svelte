<script lang="ts">
	import { getLayout } from '$lib/context/layout.svelte';
	import { type Project } from '$lib/services';
	import { X } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';
	import Files from '../edit/Files.svelte';
	import { popover } from '$lib/actions';

	interface Props {
		navBar?: boolean;
		project: Project;
		class?: string;
		currentThreadID?: string;
	}

	let { navBar = false, project, class: className, currentThreadID }: Props = $props();

	const layout = getLayout();
	let show = $derived(navBar || layout.items.length <= 1);

	const fileTT = popover({ hover: true, placement: 'top' });
</script>

{#if show}
	<div class={twMerge('flex items-start', className)}>
		{#if currentThreadID}
			<div use:fileTT.ref>
				<p use:fileTT.tooltip class="tooltip">Browse Files</p>
				<Files {project} thread {currentThreadID} primary={false} />
			</div>
		{/if}

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

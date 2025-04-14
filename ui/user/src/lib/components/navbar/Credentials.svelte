<script lang="ts">
	import { type Project } from '$lib/services';
	import Credentials from '$lib/components/edit/Credentials.svelte';
	import { X } from 'lucide-svelte/icons';
	import { clickOutside } from '$lib/actions/clickoutside';

	interface Props {
		project: Project;
	}

	let { project }: Props = $props();
	let dialog = $state<HTMLDialogElement>();
	let credentials = $state<ReturnType<typeof Credentials>>();

	export async function show() {
		await credentials?.reload();
		dialog?.showModal();
	}
</script>

<dialog
	bind:this={dialog}
	use:clickOutside={() => dialog?.close()}
	class="max-h-[90vh] min-h-[300px] w-1/3 min-w-[300px] overflow-visible p-5 pt-2"
>
	<div class="flex min-h-[300px] grow flex-col">
		<h1 class="mb-2 flex items-center justify-between text-xl font-semibold">
			Credentials
			<button class="icon-button translate-x-2" onclick={() => dialog?.close()}>
				<X class="icon-default" />
			</button>
		</h1>
		<p class="text-sm text-gray-500">These credentials are used by all threads in this Obot.</p>
		<Credentials bind:this={credentials} {project} local />
	</div>
</dialog>

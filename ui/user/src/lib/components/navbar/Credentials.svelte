<script lang="ts">
	import { type Project } from '$lib/services';
	import Credentials from '$lib/components/edit/Credentials.svelte';
	import { clickOutside } from '$lib/actions/clickoutside';
	import { responsive } from '$lib/stores';

	interface Props {
		project: Project;
		currentThreadID?: string;
	}

	let { project, currentThreadID }: Props = $props();
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
	class="max-h-full min-h-[300px] w-full max-w-full overflow-visible p-5 pt-2 md:max-h-[90vh] md:w-sm"
	class:mobile-screen-dialog={responsive.isMobile}
>
	<div class="flex h-full grow flex-col gap-4 md:h-auto md:min-h-[300px]">
		<Credentials
			bind:this={credentials}
			{project}
			local
			onClose={() => dialog?.close()}
			{currentThreadID}
		/>
	</div>
</dialog>

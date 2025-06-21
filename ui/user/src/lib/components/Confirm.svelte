<script lang="ts">
	import { clickOutside } from '$lib/actions/clickoutside';
	import { CircleAlert, LoaderCircle, X } from 'lucide-svelte/icons';

	interface Props {
		show: boolean;
		msg?: string;
		onsuccess: () => void;
		oncancel: () => void;
		loading?: boolean;
	}

	let { show = false, msg = 'OK?', onsuccess, oncancel, loading }: Props = $props();

	let dialog: HTMLDialogElement | undefined = $state();

	$effect(() => {
		if (show) {
			dialog?.showModal();
			dialog?.focus();
		} else {
			dialog?.close();
		}
	});
</script>

<dialog
	bind:this={dialog}
	use:clickOutside={() => oncancel()}
	class="max-h-full w-full max-w-md bg-gray-50 dark:bg-gray-950"
>
	<div class="relative">
		<button
			type="button"
			onclick={oncancel}
			class="absolute end-2.5 top-3 ms-auto inline-flex h-8 w-8 items-center justify-center rounded-lg bg-transparent text-sm text-black hover:bg-gray-200 dark:text-white dark:hover:bg-gray-600"
		>
			<X class="h-5 w-5" />
			<span class="sr-only">Close modal</span>
		</button>
		<div class="p-4 text-center md:p-8">
			<CircleAlert class="mx-auto mb-4 h-12 w-12 text-gray-400 dark:text-gray-100" />
			<h3 class="mb-5 text-lg font-normal break-words text-black dark:text-gray-100">{msg}</h3>
			<button
				onclick={onsuccess}
				type="button"
				class="inline-flex items-center rounded-3xl bg-red-600 px-5 py-2.5 text-center text-sm font-medium text-white hover:bg-red-800 disabled:opacity-50"
				disabled={loading}
			>
				{#if loading}
					<LoaderCircle class="size-4 animate-spin" />
				{:else}
					Yes, I'm sure
				{/if}
			</button>
			<button
				onclick={oncancel}
				type="button"
				class="ms-3 rounded-3xl bg-gray-100 px-5 py-2.5 text-sm font-medium text-black hover:bg-gray-200 dark:bg-gray-800
					 dark:text-gray-400 dark:hover:bg-gray-700 dark:hover:text-white">No, cancel</button
			>
		</div>
	</div>
</dialog>

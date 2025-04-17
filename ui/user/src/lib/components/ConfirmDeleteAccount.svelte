<script lang="ts">
	import { clickOutside } from '$lib/actions/clickoutside';
	import { CircleAlert, X } from 'lucide-svelte/icons';

	interface Props {
		show: boolean;
		msg?: string;
		username?: string;
		buttonText?: string;
		onsuccess: () => void;
		oncancel: () => void;
	}

	let {
		show = false,
		msg = '',
		username = '',
		buttonText = 'Delete',
		onsuccess,
		oncancel
	}: Props = $props();

	let dialog: HTMLDialogElement | undefined = $state();

	let username2 = $state('');

	$effect(() => {
		if (show) {
			dialog?.showModal();
			dialog?.focus();
			username2 = '';
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
		<div class="p-4 text-center md:p-5">
			<CircleAlert class="mx-auto mb-4 h-12 w-12 text-gray-400 dark:text-gray-100" />
			<h3 class="mb-5 text-lg font-normal break-words text-black dark:text-gray-100">
				{msg}
			</h3>
			<div class="mb-4">
				<p class="mb-3 text-sm font-normal text-black dark:text-gray-100">
					To confirm, type <strong>{username}</strong> in the box below
				</p>
				<input
					type="text"
					bind:value={username2}
					oninput={(e) => (username2 = (e.target as HTMLInputElement).value)}
					class="mt-1 block w-full rounded-3xl border border-gray-300 px-4 py-2 transition focus:border-blue-500 focus:ring-2 focus:ring-blue-500 focus:outline-none"
				/>
			</div>
			<button
				disabled={username2 === '' || username2 !== username}
				onclick={onsuccess}
				type="button"
				class="inline-flex w-full items-center justify-center rounded-3xl bg-red-600 px-5 py-2.5 text-center text-sm font-medium text-white hover:bg-red-800 disabled:cursor-not-allowed disabled:opacity-50"
			>
				{buttonText}
			</button>
		</div>
	</div>
</dialog>

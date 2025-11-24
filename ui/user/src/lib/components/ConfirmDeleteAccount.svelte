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
	class="dark:bg-surface1 bg-background max-h-full w-full max-w-md"
>
	<div class="relative">
		<button type="button" onclick={oncancel} class="icon-button absolute end-2.5 top-3 ms-auto">
			<X class="h-5 w-5" />
			<span class="sr-only">Close modal</span>
		</button>
		<div class="p-4 text-center md:p-5">
			<CircleAlert class="text-on-background mx-auto mb-4 h-12 w-12" />
			<h3 class="text-on-background mb-5 text-lg font-normal break-words">
				{msg}
			</h3>
			<div class="mb-4">
				<p class="text-on-background mb-3 text-sm font-normal">
					To confirm, type <strong>{username}</strong> in the box below
				</p>
				<input
					type="text"
					bind:value={username2}
					oninput={(e) => (username2 = (e.target as HTMLInputElement).value)}
					class="focus:border-primary focus:ring-primary mt-1 block w-full rounded-3xl border border-gray-300 px-4 py-2 transition focus:ring-2 focus:outline-none"
				/>
			</div>
			<button
				disabled={username2 === '' || username2 !== username}
				onclick={onsuccess}
				type="button"
				class="inline-flex w-full items-center justify-center rounded-3xl bg-red-600 px-5 py-2.5 text-center text-sm font-medium text-white hover:bg-red-800 disabled:cursor-default disabled:opacity-50"
			>
				{buttonText}
			</button>
		</div>
	</div>
</dialog>

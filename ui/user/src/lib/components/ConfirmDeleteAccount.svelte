<script lang="ts">
	import { CircleAlert, X } from 'lucide-svelte/icons';

	interface Props {
		show: boolean;
		username?: string;
		onsuccess: () => void;
		oncancel: () => void;
	}

	let { show = false, username = '', onsuccess, oncancel }: Props = $props();

	let div: HTMLDivElement | undefined = $state();

	let username2 = $state('');

	$effect(() => {
		if (show && div) {
			div.focus();
			username2 = '';
		}
	});
</script>

<div
	bind:this={div}
	tabIndex="-1"
	class:hidden={!show}
	class:flex={show}
	class="fixed top-0 right-0 left-0 z-50 h-[calc(100%-1rem)] max-h-full w-full items-center
 justify-center overflow-x-hidden overflow-y-auto bg-black/50 md:inset-0"
	role="none"
	onkeydown={(e) => {
		if (e.key === 'Escape') {
			oncancel();
		}
		e.stopPropagation();
	}}
>
	<div role="dialog" class="relative max-h-full w-full max-w-md p-4">
		<div class="relative rounded-3xl bg-gray-50 dark:bg-gray-950">
			<button
				type="button"
				onclick={oncancel}
				class="absolute end-2.5 top-3 ms-auto inline-flex h-8 w-8 items-center justify-center rounded-lg bg-transparent text-sm text-white hover:bg-gray-200 dark:hover:bg-gray-600"
			>
				<X class="h-5 w-5" />
				<span class="sr-only">Close modal</span>
			</button>
			<div class="p-4 text-center md:p-5">
				<CircleAlert class="mx-auto mb-4 h-12 w-12 text-gray-400 dark:text-gray-100" />
				<h3 class="mb-5 text-lg font-normal break-words text-black dark:text-gray-100">
					Delete {username}?
				</h3>
				<div class="mb-5">
					<p class="mb-3 text-sm font-normal text-black dark:text-gray-100">
						To confirm, type <strong>{username}</strong> in the box below
					</p>
					<input
						type="text"
						bind:value={username2}
						oninput={(e) => (username2 = (e.target as HTMLInputElement).value)}
						class="mt-1 block w-full rounded-lg border border-gray-300 px-4 py-2 transition focus:border-blue-500 focus:ring-2 focus:ring-blue-500 focus:outline-none"
					/>
				</div>
				<button
					disabled={username2 === '' || username2 !== username}
					onclick={onsuccess}
					type="button"
					class="inline-flex items-center rounded-3xl bg-red-600 px-5 py-2.5 text-center text-sm font-medium text-white hover:bg-red-800 disabled:cursor-not-allowed disabled:opacity-50"
				>
					Delete my account
				</button>
				<button
					onclick={oncancel}
					type="button"
					class="ms-3 rounded-3xl bg-gray-100 px-5 py-2.5 text-sm font-medium text-black hover:bg-gray-200 dark:bg-gray-800
					 dark:text-gray-400 dark:hover:bg-gray-700 dark:hover:text-white">Cancel</button
				>
			</div>
		</div>
	</div>
</div>

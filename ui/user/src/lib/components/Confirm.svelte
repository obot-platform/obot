<script lang="ts">
	import { CircleAlert, X } from 'lucide-svelte/icons';

	interface Props {
		show: boolean;
		msg?: string;
		onsuccess: () => void;
		oncancel: () => void;
	}

	let { show = false, msg = 'OK?', onsuccess, oncancel }: Props = $props();

	let div: HTMLDivElement | undefined = $state();

	$effect(() => {
		if (show && div) {
			div.focus();
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
				<h3 class="mb-5 text-lg font-normal text-black dark:text-gray-100">{msg}</h3>
				<button
					onclick={onsuccess}
					type="button"
					class="inline-flex items-center rounded-3xl bg-red-600 px-5 py-2.5 text-center text-sm font-medium text-white hover:bg-red-800"
				>
					Yes, I'm sure
				</button>
				<button
					onclick={oncancel}
					type="button"
					class="ms-3 rounded-3xl bg-gray-100 px-5 py-2.5 text-sm font-medium text-black hover:bg-gray-200 dark:bg-gray-800
					 dark:text-gray-400 dark:hover:bg-gray-700 dark:hover:text-white">No, cancel</button
				>
			</div>
		</div>
	</div>
</div>

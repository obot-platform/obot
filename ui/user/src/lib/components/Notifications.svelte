<script lang="ts" module>
	export class NotificationMessage {
		level: 'info' | 'error';
		message: string;

		constructor(messageOrError: string | Error, level?: 'info' | 'error') {
			if (messageOrError instanceof Error) {
				this.level = 'error';
				this.message = messageOrError.message;
			} else {
				this.level = level || 'info';
				this.message = messageOrError;
			}
		}
	}
</script>

<script lang="ts">
	import { CircleX } from '$lib/icons';
	import { X } from '$lib/icons';
	import { errors, profile } from '$lib/stores';

	let div: HTMLElement;

	$effect(() => {
		if ($profile.loaded && div.classList.contains('hidden')) {
			div.classList.remove('hidden');
			div.classList.add('flex');
		}
	});
</script>

<div bind:this={div} class="absolute bottom-0 right-0 z-50 hidden flex-col">
	{#each $errors as error, i}
		<div
			class="relative flex max-w-sm items-center gap-2 rounded-3xl bg-gray-50 p-5 dark:bg-gray-950"
		>
			<div>
				<CircleX class="h-5 w-5" />
			</div>
			<div class="pr-5 text-sm font-normal">{error.message}</div>
			<button type="button" onclick={() => errors.remove(i)} class="absolute right-0 top-0 p-5">
				<X class="h-5 w-5" />
			</button>
		</div>
	{/each}
</div>

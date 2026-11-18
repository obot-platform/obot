<script lang="ts">
	import { navigating } from '$app/state';
	import { notificationPopover } from '$lib/actions/notificationPopover';
	import { m } from '$lib/i18n';
	import { notifications, profile } from '$lib/stores';
	import Success from './Success.svelte';
	import { CircleX, Copy, X } from '@lucide/svelte';
	import { fade } from 'svelte/transition';

	let copied = $state<Record<number, boolean>>({});

	$effect(() => {
		if (navigating) {
			notifications.items = [];
			copied = {};
		}
	});
</script>

<div
	popover="manual"
	use:notificationPopover={Boolean(profile.current.loaded && notifications.items.length > 0)}
	class="notification-host flex-col gap-2"
>
	{#each notifications.items as item (item.id)}
		{#if item.kind === 'success'}
			<Success message={item.message} onClose={() => notifications.remove(item.id)} />
		{:else}
			<div
				class="relative flex max-w-sm flex-col gap-1 rounded-xl bg-gray-50 p-5 pr-12 dark:bg-gray-950"
			>
				<button
					class="group flex w-full items-center gap-2 text-left"
					onclick={() => {
						if (!navigator.clipboard) return;

						navigator.clipboard.writeText(item.message);
						copied[item.id] = true;
						setTimeout(() => {
							delete copied[item.id];
						}, 1000);
					}}
				>
					<div>
						<CircleX class="block size-5 group-hover:hidden" />
						<Copy class="hidden size-5 group-hover:block" />
					</div>
					<div class="line-clamp-3 pr-5 text-sm font-normal break-all">
						{item.message}
					</div>
				</button>
				{#if copied[item.id]}
					<div class="text-muted-content self-end text-xs" in:fade={{ duration: 200 }}>
						{m.notifications_error_copied()}
					</div>
				{/if}
				<button
					type="button"
					aria-label={m.common_close()}
					onclick={() => notifications.remove(item.id)}
					class="absolute top-0 right-0 p-4"
				>
					<X class="h-5 w-5" />
				</button>
			</div>
		{/if}
	{/each}
</div>

<style>
	.notification-host {
		position: fixed;
		inset: auto 0 0 auto;
		display: flex;
		margin: 0;
		border: none;
		padding: 0 1.25rem 1.25rem 0;
		background: transparent;
		width: max-content;
		max-width: calc(100vw - 2.5rem);
		height: fit-content;
		overflow: visible;
		color: inherit;
	}

	.notification-host:not(:popover-open) {
		display: none;
	}
</style>

<script lang="ts">
	import type { AppNotifications } from '$lib/services/admin/types';
	import { CircleAlert, Info, X } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		data?: AppNotifications['banner'];
		placeholder?: string;
	}

	let { data, placeholder }: Props = $props();
</script>

<div
	class={twMerge(
		'w-full p-2 flex items-center justify-center gap-2',
		data?.type === 'info'
			? 'bg-primary/10 text-primary'
			: 'bg-warning text-warning-content dark:bg-warning/10 dark:text-warning'
	)}
>
	<div class="flex items-center gap-1">
		{#if data?.type === 'info'}
			<Info class="size-4" />
		{:else if data?.type === 'warning'}
			<CircleAlert class="size-4" />
		{/if}
		{#if data?.text}
			<p class="text-xs font-light">{placeholder}</p>
		{:else if placeholder}
			<p class="text-xs text-muted-content/50 font-light">{data?.text}</p>
		{/if}
	</div>
	{#if data?.dismissable}
		<button
			class={twMerge(
				'btn btn-soft btn-circle btn-xs w-fit h-fit p-0.5',
				data?.type === 'info' ? 'btn-primary' : 'btn-warning'
			)}
		>
			<X class="size-3" />
		</button>
	{/if}
</div>

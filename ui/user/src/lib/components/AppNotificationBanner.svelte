<script lang="ts">
	import type { AppNotifications } from '$lib/services/user/types';
	import { CircleAlert, Info, X } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		data?: AppNotifications['banner'];
		onDismiss?: () => void;
		placeholder?: string;
	}

	let { data, onDismiss, placeholder }: Props = $props();
</script>

<div
	class={twMerge(
		'w-full py-2 px-4 flex items-center justify-center gap-2',
		data?.type === 'info'
			? 'bg-primary/10 text-primary'
			: 'bg-warning text-warning-content dark:bg-warning/10 dark:text-warning'
	)}
>
	<div class="flex items-center gap-2">
		<div class="shrink-0">
			{#if data?.type === 'info'}
				<Info class="size-4" />
			{:else if data?.type === 'warning'}
				<CircleAlert class="size-4" />
			{/if}
		</div>
		{#if data?.text}
			<p class="text-xs font-light max-w-2xl">{data.text}</p>
		{:else if placeholder}
			<p class="text-xs text-muted-content/50 font-light">{placeholder}</p>
		{/if}
	</div>
	{#if data?.dismissable}
		<button
			class={twMerge(
				'btn btn-circle btn-xs w-fit h-fit p-0.5',
				data?.type === 'info' ? 'btn-primary' : ''
			)}
			onclick={() => onDismiss?.()}
		>
			<X class="size-3" />
		</button>
	{/if}
</div>

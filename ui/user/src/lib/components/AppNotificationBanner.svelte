<script lang="ts">
	import { toHTMLFromMarkdownWithNewTabLinks } from '$lib/markdown';
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

<div class="bg-base-100 w-full min-h-8.5">
	<div
		class={twMerge(
			'w-full py-2 px-4 flex items-center justify-center gap-2',
			data?.type === 'info'
				? 'bg-primary/10 text-primary'
				: 'bg-warning text-warning-content dark:bg-warning/10 dark:text-warning'
		)}
	>
		<div class="flex items-center gap-1">
			<div class="shrink-0">
				{#if data?.type === 'info'}
					<Info class="size-4" />
				{:else if data?.type === 'warning'}
					<CircleAlert class="size-4" />
				{/if}
			</div>
			{#if data?.text}
				<div class="banner-markdown text-xs font-light max-w-2xl">
					{@html toHTMLFromMarkdownWithNewTabLinks(data.text)}
				</div>
			{:else if placeholder}
				<p
					class={twMerge(
						'text-xs font-light',
						data?.type === 'info' ? 'text-muted-content/50' : 'text-warning-content/50'
					)}
				>
					{placeholder}
				</p>
			{/if}
		</div>
		{#if data?.dismissable}
			<button
				class={twMerge(
					'btn btn-circle btn-xs w-fit h-fit p-0.5',
					data?.type === 'info' ? 'btn-primary' : ''
				)}
				onclick={() => onDismiss?.()}
				type="button"
				aria-label="Dismiss notification banner"
			>
				<X class="size-3" />
			</button>
		{/if}
	</div>
</div>

<style lang="postcss">
	.banner-markdown :global(*) {
		margin: 0;
	}

	.banner-markdown :global(a) {
		text-decoration: underline;
	}

	.banner-markdown :global(strong) {
		font-weight: 600;
	}
</style>

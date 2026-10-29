<script lang="ts">
	import { ScanEye } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	// The Supervisor's mark. Everything the Supervisor says in the UI sits next to it, so a reader
	// can tell its voice from the engine's facts at a glance.

	interface Props {
		active?: boolean;
		pulse?: boolean;
		size?: 'sm' | 'md' | 'lg';
		class?: string;
	}

	let { active = true, pulse = false, size = 'md', class: klass }: Props = $props();

	const sizes = {
		sm: { box: 'size-6', icon: 'size-3.5' },
		md: { box: 'size-8', icon: 'size-4' },
		lg: { box: 'size-12', icon: 'size-6' }
	};
</script>

<span
	class={twMerge(
		'relative inline-flex shrink-0 items-center justify-center rounded-full',
		sizes[size].box,
		active ? 'bg-primary/15 text-primary' : 'bg-base-300 dark:bg-base-400 text-muted-content',
		klass
	)}
	aria-hidden="true"
>
	{#if active && pulse}
		<span class="bg-primary/25 absolute inset-0 animate-ping rounded-full"></span>
	{/if}
	<ScanEye class={twMerge('relative', sizes[size].icon)} />
</span>

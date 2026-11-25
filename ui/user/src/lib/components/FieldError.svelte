<script lang="ts">
	import { twMerge } from 'tailwind-merge';

	interface Props {
		id: string;
		message?: string;
		class?: string;
		live?: boolean;
	}

	let { id, message, class: klass, live }: Props = $props();
</script>

{#if message}
	<p
		id={`${id}-error`}
		role={live ? 'alert' : undefined}
		class={twMerge('text-xs text-error', klass)}
	>
		{message}
	</p>
{/if}

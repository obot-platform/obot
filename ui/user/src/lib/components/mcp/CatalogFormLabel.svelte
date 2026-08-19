<script lang="ts">
	import { twMerge } from 'tailwind-merge';

	interface Props {
		title: string;
		forInput: string;
		required?: boolean;
		showError?: boolean;
	}
	let { title, forInput, required, showError }: Props = $props();
</script>

<label for={forInput} class={twMerge('text-sm font-light', showError && 'text-error')}>
	{title}
	{#if required}
		<span class={showError ? 'text-error' : ''} aria-hidden="true">*</span>
		<span class="sr-only">(required)</span>
	{/if}
</label>

<script lang="ts">
	interface Props {
		/** Tint the asterisk with the error color while the field is invalid. */
		invalid?: boolean;
		textMessage?: string;
	}

	let { invalid, textMessage }: Props = $props();
</script>

{#if textMessage}
	<span class="text-xs font-normal" class:text-error={invalid} class:text-muted-content={!invalid}>
		{textMessage}
	</span>
{:else}
	<span class="required-mark" class:text-error={invalid} aria-hidden="true"></span>
{/if}

<style>
	.required-mark::after {
		/* Fallback for browsers without alt text on `content`. */
		content: '*';
		content: '*' / '';
	}
</style>

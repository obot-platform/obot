<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		label: string;
		labelInline?: boolean;
		checked: boolean;
		disabled?: boolean;
		disablePortal?: boolean;
		onChange: (checked: boolean) => void;
		classes?: {
			label?: string;
			input?: string;
		};
	}

	let {
		label,
		labelInline,
		checked,
		disabled = false,
		onChange,
		classes,
		disablePortal
	}: Props = $props();
</script>

{#if label && !labelInline}
	<label
		class={twMerge('relative flex h-4.5 w-8.25', classes?.label)}
		use:tooltip={{ text: label, disablePortal }}
	>
		<span class="size-0 opacity-0">{label}</span>
		{@render input()}
	</label>
{:else}
	<label class={twMerge('text-muted-content flex items-center gap-1 text-xs', classes?.label)}>
		<span>{label}</span>
		<div class="relative flex h-4.5 w-8.25">
			{@render input()}
		</div>
	</label>
{/if}

{#snippet input()}
	<input
		type="checkbox"
		{checked}
		{disabled}
		class={twMerge('toggle toggle-sm', classes?.input)}
		onchange={(e) => {
			e.preventDefault();
			if (!disabled) {
				onChange(!checked);
			}
		}}
	/>
{/snippet}

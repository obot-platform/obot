<script lang="ts">
	import CopyButton from './CopyButton.svelte';
	import { Link2Icon } from '@lucide/svelte';
	import type { Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		value?: string;
		label?: string;
		id: string;
		preContent?: Snippet;
		class?: string;
		classes?: {
			inputLabel?: string;
			input?: string;
		};
		variant?: 'code' | 'default';
	}

	let {
		value,
		label,
		id,
		preContent,
		class: klass,
		classes,
		variant = 'default'
	}: Props = $props();

	let copyButton = $state<ReturnType<typeof CopyButton>>();

	export function clear() {
		copyButton?.clearButtonText();
	}
</script>

{#if label}
	<label class="label" for={id}>
		{label}
	</label>
{/if}
<div
	class={twMerge(
		'rounded-field bg-base-200 border-none input w-full px-0',
		variant === 'code' && 'pl-2.5',
		klass
	)}
>
	{#if variant !== 'code'}
		<div
			class={twMerge(
				'label px-2.5 flex items-center gap-2 text-xs text-base-content/75 shrink-0 ml-1 mr-0 ',
				classes?.inputLabel
			)}
		>
			{#if preContent}
				{@render preContent?.()}
			{:else}
				<Link2Icon class="size-4" />
			{/if}
		</div>
	{/if}
	<input
		onmousedown={() => copyButton?.copy()}
		type="text"
		value={value ?? ''}
		class={twMerge('w-full text-xs', variant === 'code' && 'font-mono', classes?.input)}
		readonly
		{id}
	/>
	<div class="mr-2">
		<CopyButton
			bind:this={copyButton}
			classes={{
				button:
					'shrink-0 text-xs flex gap-1 :not([disabled]):hover:text-base-content :not([disabled]):text-base-content/65 disabled:cursor-not-allowed'
			}}
			text={value ?? ''}
			showTextLeft
		/>
	</div>
</div>

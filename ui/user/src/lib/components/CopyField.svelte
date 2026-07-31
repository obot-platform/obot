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
	let labelId = $derived(`${id}-label`);

	export function clear() {
		copyButton?.clearButtonText();
	}
</script>

{#if label}
	{#if variant === 'code'}
		<div class="label" id={labelId}>{label}</div>
	{:else}
		<label class="label" for={id}>
			{label}
		</label>
	{/if}
{/if}
<div
	class={twMerge(
		'copy-field-input rounded-field bg-base-200 border-none input w-full px-0',
		variant === 'code' && 'h-auto min-h-10 items-start py-2 pl-2.5',
		klass
	)}
>
	{#if variant !== 'code' || preContent}
		<div
			class={twMerge(
				'label px-2.5 flex items-center gap-2 text-xs text-base-content/75 shrink-0 ml-1 mr-0 ',
				variant === 'code' && 'pl-0 pr-2.5 mt-0.5',
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
	{#if variant === 'code'}
		<pre
			{id}
			aria-labelledby={label ? labelId : undefined}
			class={twMerge(
				'm-0 w-full overflow-x-auto font-mono text-xs whitespace-pre-wrap break-all',
				classes?.input
			)}><code class="font-mono text-xs">{value ?? ''}</code></pre>
	{:else}
		<input
			onmousedown={() => copyButton?.copy()}
			type="text"
			value={value ?? ''}
			class={twMerge('w-full text-xs', classes?.input)}
			readonly
			{id}
		/>
	{/if}
	<div class={twMerge('mr-2', variant === 'code' && 'mt-0.5')}>
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

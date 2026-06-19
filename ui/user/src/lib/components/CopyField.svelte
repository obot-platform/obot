<script lang="ts">
	import CopyButton from './CopyButton.svelte';
	import { Link2Icon } from '@lucide/svelte';

	interface Props {
		value?: string;
		label?: string;
		id: string;
	}

	let { value, label, id }: Props = $props();

	let copyButton = $state<ReturnType<typeof CopyButton>>();
</script>

{#if label}
	<label class="label" for={id}>
		{label}
	</label>
{/if}
<div class="rounded-field bg-base-200 border-none input w-full pr-0">
	<div class="label px-2.5 flex items-center gap-2 text-xs text-base-content/75 shrink-0 mr-1">
		<Link2Icon class="size-4" />
	</div>
	<input
		onmousedown={() => copyButton?.copy()}
		type="text"
		value={value ?? ''}
		class="w-full text-xs"
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

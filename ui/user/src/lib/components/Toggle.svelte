<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		label: string;
		labelInline?: boolean;
		checked: boolean;
		disabled?: boolean;
		onChange: (checked: boolean) => void;
		classes?: {
			label?: string;
			input?: string;
		};
	}

	let { label, labelInline, checked, disabled = false, onChange, classes }: Props = $props();
</script>

{#if label && !labelInline}
	<label
		class={twMerge('relative flex h-4.5 w-8.25', classes?.label)}
		use:tooltip={{ text: label, disablePortal: true }}
	>
		<span class="size-0 opacity-0">{label}</span>
		{@render input()}
	</label>
{:else}
	<label class={twMerge('flex items-center gap-1 text-xs', classes?.label)}>
		<span class="text-gray-500">{label}</span>
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
		class={twMerge('opacity-0', classes?.input)}
		readonly
		onchange={(e) => {
			e.preventDefault();
			if (!disabled) {
				onChange(!checked);
			}
		}}
	/>
	<span class="slider rounded-2xl" class:checked class:disabled></span>
{/snippet}

<style lang="postcss">
	/* The slider */
	:global {
		.slider {
			position: absolute;
			cursor: pointer;
			top: 0;
			left: 0;
			right: 0;
			bottom: 0;
			background-color: var(--color-surface3);
			-webkit-transition: 0.4s;
			transition: 0.4s;

			.dark & {
				&::before {
					background-color: var(--color-surface1);
				}
			}
		}

		.slider.disabled {
			cursor: not-allowed;
			opacity: 0.6;
		}

		.slider:before {
			position: absolute;
			content: '';
			height: 0.825rem;
			width: 0.825rem;
			left: 0.145rem;
			bottom: 0.145rem;
			background-color: var(--color-white);
			-webkit-transition: 0.4s;
			transition: 0.4s;
			border-radius: 50%;
		}

		.slider.checked {
			background-color: var(--color-blue-500);
		}

		.slider.checked:before {
			-webkit-transform: translateX(0.925rem);
			-ms-transform: translateX(0.925rem);
			transform: translateX(0.925rem);
		}

		input:focus + .slider {
			box-shadow: 0 0 1px var(--color-blue-500);
		}
	}
</style>

<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';

	interface Props {
		label: string;
		checked: boolean;
		onChange: (checked: boolean) => void;
	}

	let { label, checked, onChange }: Props = $props();
	$effect(() => {
		console.log(checked);
	});
</script>

<label class="relative flex h-4.5 w-8.25" use:tooltip={label}>
	<input
		type="checkbox"
		{checked}
		class="opacity-0"
		readonly
		onchange={(e) => {
			e.preventDefault();
			onChange(!checked);
		}}
	/>
	<span class="slider rounded-2xl" class:checked></span>
</label>

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

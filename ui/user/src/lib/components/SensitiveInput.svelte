<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { Eye, EyeOff } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		name: string;
		value?: string;
		error?: boolean;
		oninput?: () => void;
		textarea?: boolean;
		disabled?: boolean;
	}

	let { name, value = $bindable(''), error, oninput, textarea, disabled }: Props = $props();
	let showSensitive = $state(false);
	let textareaElement = $state<HTMLTextAreaElement>();
	let isPulsing = $state(false);

	function getMaskedValue(text: string): string {
		return text.replace(/[^\n\r]/g, '•');
	}

	function handleInput(event: Event) {
		const input = event.target as HTMLInputElement;
		value = input.value;
		oninput?.();
	}

	function toggleVisibility(e: MouseEvent) {
		e.preventDefault();
		showSensitive = !showSensitive;

		if (showSensitive) {
			textareaElement?.focus();
		}

		isPulsing = false;
	}
</script>

<div class="relative flex grow items-center">
	{#if textarea}
		<textarea
			bind:this={textareaElement}
			data-1p-ignore
			id={name}
			{name}
			{disabled}
			class={twMerge(
				'text-input-filled w-full pr-10',
				error && 'border-red-500 bg-red-500/20 ring-red-500 focus:ring-1'
			)}
			class:text-red-500={error}
			bind:value={
				() => (showSensitive ? value : getMaskedValue(value || '')),
				(v) => {
					value = v;
					oninput?.();
				}
			}
			onkeydown={(ev) => {
				if (!showSensitive) {
					if (ev.key === 'v' && (ev.metaKey || ev.ctrlKey)) return true;

					ev.preventDefault();
					isPulsing = true;

					return false;
				}
			}}
		></textarea>
	{:else}
		<input
			data-1p-ignore
			id={name}
			{name}
			class={twMerge(
				'text-input-filled w-full pr-10',
				error && 'border-red-500 bg-red-500/20 ring-red-500 focus:ring-1'
			)}
			class:text-red-500={error}
			{value}
			type={showSensitive ? 'text' : 'password'}
			oninput={handleInput}
			autocomplete="new-password"
			{disabled}
		/>
	{/if}

	<div
		class="absolute top-1/2 right-4 z-10 grid -translate-y-1/2 grid-cols-1 grid-rows-1"
		use:tooltip={{ disablePortal: true, text: showSensitive ? 'Hide' : 'Reveal' }}
	>
		<button
			type="button"
			class="cursor-pointer transition-colors duration-150"
			class:text-red-500={error}
			class:pulse={isPulsing}
			onclick={toggleVisibility}
		>
			{#if showSensitive}
				<EyeOff class="size-4" />
			{:else}
				<Eye class="size-4" />
			{/if}
		</button>
	</div>
</div>

<style>
	@keyframes pulse {
		0% {
			color: rgb(255, 255, 255);
			transform: scale(1);
		}

		100% {
			color: var(--color-blue);
			transform: scale(1.2);
		}
	}

	.pulse {
		animation: pulse 0.2s ease-in-out alternate infinite;
	}
</style>

<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { Eye, EyeOff } from 'lucide-svelte';

	interface Props {
		name: string;
		value?: string;
	}

	let { name, value = $bindable('') }: Props = $props();
	let showSensitive = $state(false);

	function handleInput(event: Event) {
		const input = event.target as HTMLInputElement;
		value = input.value;
	}

	function toggleVisibility(e: MouseEvent) {
		e.preventDefault();
		showSensitive = !showSensitive;
	}
</script>

<div class="relative flex grow items-center">
	<input
		data-1p-ignore
		id={name}
		{name}
		class="text-input-filled w-full pr-10"
		{value}
		type={showSensitive ? 'text' : 'password'}
		oninput={handleInput}
		autocomplete="new-password"
	/>

	<button
		class="absolute top-1/2 right-4 z-10 -translate-y-1/2 cursor-pointer"
		onclick={toggleVisibility}
		use:tooltip={{ disablePortal: true, text: showSensitive ? 'Hide' : 'Reveal' }}
	>
		{#if showSensitive}
			<EyeOff class="size-4" />
		{:else}
			<Eye class="size-4" />
		{/if}
	</button>
</div>

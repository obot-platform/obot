<script lang="ts">
	import { clickOutside } from '$lib/actions/clickoutside';
	import type { BaseProvider } from '$lib/services/admin/types';
	import { darkMode, responsive } from '$lib/stores';
	import { ChevronRight, X } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';
	import SensitiveInput from '../SensitiveInput.svelte';

	interface Props {
		provider?: BaseProvider;
	}

	const { provider }: Props = $props();
	let dialog = $state<HTMLDialogElement>();

	export function open() {
		dialog?.showModal();
	}

	export function close() {
		dialog?.close();
	}
</script>

<dialog
	bind:this={dialog}
	class="w-full max-w-2xl"
	class:p-4={!responsive.isMobile}
	class:mobile-screen-dialog={responsive.isMobile}
	use:clickOutside={() => close()}
>
	{#if provider}
		<div class="flex flex-col gap-4">
			<h3 class="default-dialog-title" class:default-dialog-mobile-title={responsive.isMobile}>
				<span class="flex items-center gap-2">
					{#if darkMode.isDark}
						{@const url = provider.iconDark ?? provider.icon}
						<img
							src={url}
							alt={provider.name}
							class={twMerge('size-6 rounded-md p-1', !provider.iconDark && 'bg-gray-600')}
						/>
					{:else}
						<img src={provider.icon} alt={provider.name} class="size-6 rounded-md p-1" />
					{/if}
					{provider.name}
				</span>
				<button
					class:mobile-header-button={responsive.isMobile}
					onclick={() => close()}
					class="icon-button"
				>
					{#if responsive.isMobile}
						<ChevronRight class="size-6" />
					{:else}
						<X class="size-5" />
					{/if}
				</button>
			</h3>
			{#if provider.requiredConfigurationParameters && provider.requiredConfigurationParameters.length > 0}
				<div class="flex flex-col gap-4">
					<h4 class="text-lg font-semibold">Required Configuration</h4>
					<ul class="flex flex-col gap-4">
						{#each provider.requiredConfigurationParameters as parameter}
							<li class="flex flex-col gap-1">
								<label for={parameter.name}>{parameter.friendlyName}</label>
								{#if parameter.sensitive}
									<SensitiveInput name={parameter.name} />
								{:else}
									<input type="text" id={parameter.name} class="text-input-filled" />
								{/if}
							</li>
						{/each}
					</ul>
				</div>
			{/if}
			{#if provider.optionalConfigurationParameters && provider.optionalConfigurationParameters.length > 0}
				<div class="flex flex-col gap-2">
					<h4 class="text-lg font-semibold">Optional Configuration</h4>
					<ul class="flex flex-col gap-4">
						{#each provider.optionalConfigurationParameters as parameter}
							<li class="flex flex-col gap-1">
								<label for={parameter.name}>{parameter.friendlyName}</label>
								<input type="text" id={parameter.name} class="text-input-filled" />
							</li>
						{/each}
					</ul>
				</div>
			{/if}
		</div>
	{/if}
</dialog>

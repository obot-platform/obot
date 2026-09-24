<script lang="ts">
	import Toggle from '../Toggle.svelte';
	import { slide } from 'svelte/transition';

	interface Props {
		config: { localhostCallbackEnabled?: boolean; localhostCallbackPath?: string };
		readonly?: boolean;
	}
	let { config = $bindable(), readonly = false }: Props = $props();
	const uid = $props.id();
</script>

<div
	class="dark:bg-base-200 dark:border-base-400 bg-base-100 flex flex-col gap-4 rounded-lg border border-transparent p-4 shadow-sm"
>
	<div class="flex justify-between gap-4">
		<button
			type="button"
			class="flex grow cursor-pointer flex-col gap-1 text-left"
			disabled={readonly}
			onclick={() => {
				config = { ...config, localhostCallbackEnabled: !config.localhostCallbackEnabled };
			}}
		>
			<h4 class="text-sm font-semibold" class:opacity-50={!config.localhostCallbackEnabled}>
				Localhost OAuth callback
			</h4>
			<p class="text-muted-content text-xs font-light">
				For providers that require a localhost callback. Connect using <code>obot mcp connect</code>
				on the same computer as your browser. Obot continues managing upstream credentials.
			</p>
		</button>
		<div class="flex self-start">
			<Toggle
				classes={{ label: 'text-sm text-inherit' }}
				label={config.localhostCallbackEnabled
					? 'Disable Localhost OAuth callback'
					: 'Enable Localhost OAuth callback'}
				checked={!!config.localhostCallbackEnabled}
				disabled={readonly}
				onChange={(checked) => {
					config = { ...config, localhostCallbackEnabled: checked };
				}}
			/>
		</div>
	</div>
	{#if config.localhostCallbackEnabled}
		<div class="flex flex-col gap-2" in:slide={{ axis: 'y' }}>
			<label for={`${uid}-callback-path`} class="text-sm font-light">Callback path</label>
			<input
				id={`${uid}-callback-path`}
				class="text-input-filled dark:bg-base-100 w-full"
				value={config.localhostCallbackPath ?? ''}
				oninput={(event) => {
					config = { ...config, localhostCallbackPath: event.currentTarget.value };
				}}
				disabled={readonly}
				placeholder="/oauth/callback"
			/>
			<p class="text-muted-content text-xs font-light">Leave blank for /oauth/callback.</p>
		</div>
	{/if}
</div>

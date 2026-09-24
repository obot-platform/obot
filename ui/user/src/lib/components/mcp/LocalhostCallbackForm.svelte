<script lang="ts">
	interface Props {
		config: { localhostCallbackEnabled?: boolean; localhostCallbackPath?: string };
		readonly?: boolean;
	}
	let { config = $bindable(), readonly = false }: Props = $props();
	const uid = $props.id();
</script>

<div class="bg-base-200 flex flex-col gap-3 rounded-lg p-4">
	<label class="flex items-center gap-2 text-sm font-semibold">
		<input
			type="checkbox"
			class="checkbox checkbox-sm"
			checked={!!config.localhostCallbackEnabled}
			onchange={(event) => {
				config = { ...config, localhostCallbackEnabled: event.currentTarget.checked };
			}}
			disabled={readonly}
		/>
		Localhost OAuth callback
	</label>
	<p class="text-muted-content text-xs">
		For providers that require a localhost callback. Connect using <code>obot mcp connect</code>
		on the same computer as your browser. Obot continues managing upstream credentials.
	</p>
	{#if config.localhostCallbackEnabled}
		<label for={`${uid}-callback-path`} class="text-sm">Callback path</label>
		<input
			id={`${uid}-callback-path`}
			class="input w-full"
			value={config.localhostCallbackPath ?? ''}
			oninput={(event) => {
				config = { ...config, localhostCallbackPath: event.currentTarget.value };
			}}
			disabled={readonly}
			placeholder="/oauth/callback"
		/>
		<p class="text-muted-content text-xs">Leave blank for /oauth/callback.</p>
	{/if}
</div>

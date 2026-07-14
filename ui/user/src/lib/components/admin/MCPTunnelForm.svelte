<script lang="ts">
	import Loading from '$lib/icons/Loading.svelte';
	import { AdminService, type MCPTunnel, type MCPTunnelManifest } from '$lib/services';
	import { Plus, RefreshCw, Trash2 } from '@lucide/svelte';
	import { untrack } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		onCreate?: (tunnel: MCPTunnel) => void;
		onDelete?: () => void;
		onRotateSecret?: () => void;
		onUpdate?: (tunnel: MCPTunnel) => void;
		readonly?: boolean;
		tunnel?: MCPTunnel;
	}

	type MCPTunnelFormManifest = Omit<MCPTunnelManifest, 'allowedURLs'> & {
		allowedURLs: string[];
	};

	let { onCreate, onDelete, onRotateSecret, onUpdate, readonly, tunnel }: Props = $props();

	let manifest = $state<MCPTunnelFormManifest>(
		untrack(() => ({
			allowedURLs: [...(tunnel?.manifest.allowedURLs ?? [])],
			description: tunnel?.manifest.description ?? '',
			displayName: tunnel?.manifest.displayName ?? ''
		}))
	);
	let saving = $state(false);
	let showErrors = $state(false);

	let initialManifest = $derived(
		JSON.stringify({
			allowedURLs: tunnel?.manifest.allowedURLs ?? [],
			description: tunnel?.manifest.description ?? '',
			displayName: tunnel?.manifest.displayName ?? ''
		})
	);
	let currentManifest = $derived(
		JSON.stringify({
			allowedURLs: manifest.allowedURLs,
			description: manifest.description ?? '',
			displayName: manifest.displayName ?? ''
		})
	);
	let hasChanges = $derived(initialManifest !== currentManifest);

	function allowedURLValidation(value: string): string {
		const candidate = value.trim();
		if (!candidate) {
			return 'Allowed URL is required';
		}

		const wildcardCount = candidate.split('*').length - 1;
		if (wildcardCount > 1) {
			return 'Use at most one wildcard';
		}
		if (wildcardCount === 1 && !candidate.startsWith('*') && !candidate.endsWith('*')) {
			return 'The wildcard must be at the beginning or end';
		}

		return '';
	}

	let displayNameError = $derived(
		showErrors && !manifest.displayName.trim() ? 'Display name is required' : ''
	);
	let allowedURLErrors = $derived(
		manifest.allowedURLs.map((value) => (showErrors ? allowedURLValidation(value) : ''))
	);

	function normalizedManifest(): MCPTunnelManifest {
		return {
			allowedURLs: manifest.allowedURLs.map((value) => value.trim()),
			description: manifest.description?.trim() || undefined,
			displayName: manifest.displayName.trim()
		};
	}

	async function save() {
		if (readonly) return;
		showErrors = true;

		if (
			!manifest.displayName.trim() ||
			manifest.allowedURLs.some((value) => allowedURLValidation(value))
		) {
			return;
		}

		saving = true;
		try {
			const saved = tunnel
				? await AdminService.updateMCPTunnel(tunnel.id, normalizedManifest())
				: await AdminService.createMCPTunnel(normalizedManifest());

			manifest = {
				allowedURLs: [...(saved.manifest.allowedURLs ?? [])],
				description: saved.manifest.description ?? '',
				displayName: saved.manifest.displayName
			};
			showErrors = false;

			if (tunnel) {
				onUpdate?.(saved);
			} else {
				onCreate?.(saved);
			}
		} finally {
			saving = false;
		}
	}
</script>

<form
	class="flex flex-col gap-6"
	onsubmit={(event) => {
		event.preventDefault();
		save();
	}}
>
	<div
		class="dark:bg-base-200 dark:border-base-400 bg-base-100 flex flex-col gap-6 rounded-lg border border-transparent p-4 shadow-sm"
	>
		<div class="flex flex-col gap-2">
			<label for="mcp-tunnel-display-name" class="text-sm font-light">
				Display Name
				{#if !readonly}
					<span class="text-error" aria-hidden="true">*</span>
				{/if}
			</label>
			<input
				id="mcp-tunnel-display-name"
				class={twMerge('text-input-filled dark:bg-base-100', displayNameError && 'error')}
				bind:value={manifest.displayName}
				disabled={readonly}
				aria-invalid={displayNameError ? 'true' : undefined}
				aria-describedby={displayNameError ? 'mcp-tunnel-display-name-error' : undefined}
				oninput={() => {
					showErrors = false;
				}}
			/>
			{#if displayNameError}
				<p id="mcp-tunnel-display-name-error" class="text-error text-xs" role="alert">
					{displayNameError}
				</p>
			{/if}
		</div>

		<div class="flex flex-col gap-2">
			<label for="mcp-tunnel-description" class="text-sm font-light">Description</label>
			<textarea
				id="mcp-tunnel-description"
				class="text-input-filled dark:bg-base-100 min-h-28 resize-y"
				bind:value={manifest.description}
				disabled={readonly}
				placeholder="Describe where this tunnel connects."
			></textarea>
		</div>

		<div class="flex flex-col gap-3">
			<div class="flex flex-col gap-1">
				<span class="text-sm font-light">Allowed URLs</span>
				<p class="text-muted-content text-xs font-light">
					Add exact URLs or hostnames. Use a trailing <code>*</code> for a prefix or a leading
					<code>*</code> for a suffix.
				</p>
			</div>

			{#each manifest.allowedURLs as _, index (index)}
				<div class="flex items-start gap-2">
					<div class="flex grow flex-col gap-1">
						<input
							id={`mcp-tunnel-allowed-url-${index}`}
							class={twMerge(
								'text-input-filled dark:bg-base-100',
								allowedURLErrors[index] && 'error'
							)}
							bind:value={manifest.allowedURLs[index]}
							disabled={readonly}
							placeholder="e.g. https://api.internal/* or *.internal"
							aria-invalid={allowedURLErrors[index] ? 'true' : undefined}
							oninput={() => {
								showErrors = false;
							}}
						/>
						{#if allowedURLErrors[index]}
							<p class="text-error text-xs" role="alert">{allowedURLErrors[index]}</p>
						{/if}
					</div>
					{#if !readonly}
						<button
							type="button"
							class="btn btn-square btn-secondary"
							aria-label={`Delete allowed URL ${index + 1}`}
							onclick={() => {
								manifest.allowedURLs.splice(index, 1);
							}}
						>
							<Trash2 class="size-4" />
						</button>
					{/if}
				</div>
			{/each}

			{#if !readonly}
				<button
					type="button"
					class="btn btn-secondary btn-sm flex w-fit items-center gap-1"
					onclick={() => {
						manifest.allowedURLs.push('');
					}}
				>
					<Plus class="size-4" />
					Allowed URL
				</button>
			{/if}
		</div>
	</div>

	{#if tunnel}
		<div
			class="dark:bg-base-200 dark:border-base-400 bg-base-100 flex flex-col gap-4 rounded-lg border border-transparent p-4 shadow-sm"
		>
			<h2 class="text-sm font-semibold">Tunnel Credentials</h2>
			<div class="grid gap-4 md:grid-cols-2">
				<div class="flex min-w-0 flex-col gap-1">
					<span class="text-muted-content text-xs">Tunnel ID</span>
					<code class="truncate text-sm" title={tunnel.id}>{tunnel.id}</code>
				</div>
				<div class="flex min-w-0 flex-col gap-1">
					<span class="text-muted-content text-xs">Secret Preview</span>
					<code class="truncate text-sm" title={tunnel.token}>{tunnel.token}</code>
				</div>
			</div>
			<p class="text-muted-content text-xs font-light">
				The complete secret is only shown when the tunnel is created or its secret is rotated.
			</p>
		</div>
	{/if}

	{#if !readonly}
		<div class="flex flex-wrap items-center justify-between gap-3">
			<div class="flex flex-wrap gap-2">
				{#if tunnel}
					<button
						type="button"
						class="btn btn-secondary flex items-center gap-1"
						onclick={onRotateSecret}
					>
						<RefreshCw class="size-4" />
						Rotate Secret
					</button>
					<button type="button" class="btn btn-error flex items-center gap-1" onclick={onDelete}>
						<Trash2 class="size-4" />
						Delete
					</button>
				{/if}
			</div>

			<button
				type="submit"
				class="btn btn-primary flex items-center gap-1"
				disabled={saving || !hasChanges}
			>
				{#if saving}
					<Loading class="size-4" />
				{/if}
				{tunnel ? 'Save Changes' : 'Create Tunnel'}
			</button>
		</div>
	{/if}
</form>

<script lang="ts">
	import { FORM_PARENT_ROUTES } from '$lib/constants';
	import { FormValidation } from '$lib/form.svelte';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import { AdminService, type MCPTunnel, type MCPTunnelManifest } from '$lib/services';
	import { goto } from '$lib/url';
	import FieldError from '../FieldError.svelte';
	import RequiredMark from '../RequiredMark.svelte';
	import { Plus, RefreshCw, Trash2 } from '@lucide/svelte';
	import { untrack } from 'svelte';

	interface Props {
		onCancel?: () => void;
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

	let { onCancel, onCreate, onDelete, onRotateSecret, onUpdate, readonly, tunnel }: Props =
		$props();

	let manifest = $state<MCPTunnelFormManifest>(
		untrack(() => ({
			allowedURLs: [...(tunnel?.manifest.allowedURLs ?? [])],
			description: tunnel?.manifest.description ?? '',
			displayName: tunnel?.manifest.displayName ?? ''
		}))
	);
	let saving = $state(false);

	function allowedURLValidation(value: string): string {
		const candidate = value.trim();
		if (!candidate) {
			return m.mcps_tunnels_allowed_url_required();
		}

		const wildcardCount = candidate.split('*').length - 1;
		if (wildcardCount > 1) {
			return m.mcps_tunnels_one_wildcard();
		}
		if (wildcardCount === 1 && !candidate.startsWith('*') && !candidate.endsWith('*')) {
			return m.mcps_tunnels_wildcard_position();
		}

		return '';
	}

	const validation = new FormValidation<string>(() => {
		const errors: Record<string, string> = {};
		if (!manifest.displayName.trim()) {
			errors.displayName = m.mcps_tunnels_display_name_required();
		}
		manifest.allowedURLs.forEach((value, index) => {
			const error = allowedURLValidation(value);
			if (error) errors[`allowedURL.${index}`] = error;
		});
		return errors;
	});

	function cancel() {
		if (onCancel) onCancel();
		else goto(FORM_PARENT_ROUTES.tunnels);
	}

	function normalizedManifest(): MCPTunnelManifest {
		return {
			allowedURLs: manifest.allowedURLs.map((value) => value.trim()),
			description: manifest.description?.trim() || undefined,
			displayName: manifest.displayName.trim()
		};
	}

	async function save() {
		if (readonly) return;
		if (!(await validation.check())) return;

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
			validation.reset();

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
	{@attach validation.attach}
	class="flex flex-col gap-6"
	novalidate
	onsubmit={(event) => {
		event.preventDefault();
		save();
	}}
>
	<div
		class="dark:bg-base-200 dark:border-base-400 bg-base-100 flex flex-col gap-6 rounded-lg border border-transparent p-4 shadow-sm"
	>
		<div class="flex flex-col gap-2">
			<label
				for="mcp-tunnel-display-name"
				class="text-sm font-light"
				class:error={validation.has('displayName')}
			>
				{m.mcps_tunnels_display_name()}
				{#if !readonly}
					<RequiredMark invalid={validation.has('displayName')} />
				{/if}
			</label>
			<input
				class="text-input-filled dark:bg-base-100"
				class:error={validation.has('displayName')}
				{...validation.attrs('displayName', 'mcp-tunnel-display-name')}
				aria-required="true"
				bind:value={manifest.displayName}
				disabled={readonly}
			/>
			<FieldError id="mcp-tunnel-display-name" message={validation.message('displayName')} />
		</div>

		<div class="flex flex-col gap-2">
			<label for="mcp-tunnel-description" class="text-sm font-light">{m.core_description()}</label>
			<textarea
				id="mcp-tunnel-description"
				class="text-input-filled dark:bg-base-100 min-h-28 resize-y"
				bind:value={manifest.description}
				disabled={readonly}
				placeholder={m.mcps_tunnels_description_placeholder()}></textarea>
		</div>

		<div class="flex flex-col gap-3">
			<div class="flex flex-col gap-1">
				<span
					class="text-sm font-light"
					class:text-error={manifest.allowedURLs.some((_, i) => validation.has(`allowedURL.${i}`))}
					>{m.mcps_tunnels_allowed_urls()}
					<RequiredMark
						invalid={manifest.allowedURLs.some((_, i) => validation.has(`allowedURL.${i}`))}
					/></span
				>
				<p class="text-muted-content text-xs font-light">
					{m.mcps_tunnels_allowed_urls_hint_prefix()}
					<code>*</code>
					{m.mcps_tunnels_allowed_urls_hint_middle()}
					<code>*</code>
					{m.mcps_tunnels_allowed_urls_hint_suffix()}
				</p>
			</div>

			{#each manifest.allowedURLs as _, index (index)}
				<div class="flex items-start gap-2">
					<div class="flex grow flex-col gap-1">
						<input
							class="text-input-filled dark:bg-base-100"
							class:error={validation.has(`allowedURL.${index}`)}
							{...validation.attrs(`allowedURL.${index}`, `mcp-tunnel-allowed-url-${index}`)}
							aria-required="true"
							bind:value={manifest.allowedURLs[index]}
							disabled={readonly}
							placeholder={m.mcps_tunnels_allowed_url_placeholder()}
						/>
						<FieldError
							id={`mcp-tunnel-allowed-url-${index}`}
							message={validation.message(`allowedURL.${index}`)}
						/>
					</div>
					{#if !readonly}
						<button
							type="button"
							class="btn btn-square btn-secondary"
							aria-label={m.mcps_tunnels_delete_allowed_url({ n: index + 1 })}
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
					{m.mcps_tunnels_allowed_url()}
				</button>
			{/if}
		</div>
	</div>

	{#if tunnel}
		<div
			class="dark:bg-base-200 dark:border-base-400 bg-base-100 flex flex-col gap-4 rounded-lg border border-transparent p-4 shadow-sm"
		>
			<h2 class="text-sm font-semibold">{m.mcps_tunnels_credentials()}</h2>
			<div class="grid gap-4 md:grid-cols-2">
				<div class="flex min-w-0 flex-col gap-1">
					<span class="text-muted-content text-xs">{m.mcps_tunnels_tunnel_id()}</span>
					<code class="truncate text-sm" title={tunnel.id}>{tunnel.id}</code>
				</div>
				<div class="flex min-w-0 flex-col gap-1">
					<span class="text-muted-content text-xs">{m.mcps_tunnels_secret_preview()}</span>
					<code class="truncate text-sm" title={tunnel.token}>{tunnel.token}</code>
				</div>
			</div>
			<p class="text-muted-content text-xs font-light">
				{m.mcps_tunnels_secret_shown_once()}
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
						{m.mcps_tunnels_rotate_secret()}
					</button>
					<button type="button" class="btn btn-error flex items-center gap-1" onclick={onDelete}>
						<Trash2 class="size-4" />
						{m.core_delete()}
					</button>
				{/if}
			</div>

			<div class="flex gap-2">
				<button type="button" class="btn btn-secondary" onclick={cancel}>
					{m.common_cancel()}
				</button>
				<button type="submit" class="btn btn-primary flex items-center gap-1" disabled={saving}>
					{#if saving}
						<Loading class="size-4" />
					{/if}
					{tunnel ? m.core_save_changes() : m.mcps_tunnels_create_tunnel()}
				</button>
			</div>
		</div>
	{/if}
</form>

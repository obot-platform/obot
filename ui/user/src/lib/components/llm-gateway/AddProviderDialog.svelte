<script lang="ts">
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import SensitiveInput from '$lib/components/SensitiveInput.svelte';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		createProvider,
		createProviderKey,
		deleteProvider,
		type Provider
	} from '$lib/services/llm-gateway/gateway';
	import {
		CHAT_COMPLETIONS_ONLY_REQUESTS,
		CUSTOM_PROVIDER_BASES,
		PROVIDER_CATALOG,
		catalogEntry,
		setPath,
		type ProviderCatalogEntry
	} from '$lib/services/llm-gateway/providers';
	import { Blocks, ChevronLeft } from '@lucide/svelte';

	interface Props {
		configured: string[];
		existingKeyNames: string[];
		onCreated?: (provider: string) => void;
	}

	let { configured, existingKeyNames, onCreated }: Props = $props();

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let selected = $state<ProviderCatalogEntry | 'custom'>();
	let search = $state('');
	let values = $state<Record<string, string>>({});
	let baseURL = $state('');
	let customName = $state('');
	let customBase = $state('openai');
	let keyless = $state(false);
	// Whether a custom OpenAI-compatible server serves the Responses API as well as Chat
	// Completions. Many self-hosted servers serve only Chat Completions.
	let responsesAPI = $state(false);
	let allowPrivate = $state(false);
	let saving = $state(false);
	let error = $state('');
	// Opened for one provider, from its card, rather than to pick one.
	let direct = $state(false);

	// Opens the dialog to pick a provider, or straight to the form for one: a catalog
	// provider's ID, or 'custom'.
	export function open(provider?: string) {
		const entry = provider && provider !== 'custom' ? catalogEntry(provider) : undefined;
		selected = provider === 'custom' ? 'custom' : entry;
		direct = !!selected;
		search = '';
		reset();
		dialog?.open();
	}

	function reset() {
		values = {};
		baseURL = '';
		customName = '';
		customBase = 'openai';
		keyless = false;
		responsesAPI = false;
		allowPrivate = false;
		error = '';
	}

	let catalog = $derived(
		PROVIDER_CATALOG.filter(
			(p) =>
				!configured.includes(p.id) &&
				(!search ||
					p.label.toLowerCase().includes(search.toLowerCase()) ||
					p.id.includes(search.toLowerCase()))
		)
	);

	let fields = $derived(
		selected === 'custom'
			? keyless
				? []
				: [{ path: 'value', label: m.models_providers_api_key(), sensitive: true, required: false }]
			: (selected?.keyFields ?? [])
	);

	let providerID = $derived(selected === 'custom' ? customName.trim() : (selected?.id ?? ''));

	let valid = $derived(
		!!providerID &&
			(selected !== 'custom' ||
				(/^[a-z0-9][a-z0-9_-]*$/.test(providerID) &&
					!configured.includes(providerID) &&
					!!baseURL.trim())) &&
			fields.every((f) => !('required' in f) || !f.required || !!values[f.path]?.trim())
	);

	function keyName() {
		for (let i = 1; ; i++) {
			const candidate = `${providerID}-key-${i}`;
			if (!existingKeyNames.includes(candidate)) return candidate;
		}
	}

	async function save() {
		saving = true;
		error = '';
		let created = false;
		try {
			const body: Partial<Provider> & { provider: string } = {
				provider: providerID,
				network_config: {
					default_request_timeout_in_seconds: 300,
					// Retries also let requests fail over between the provider's keys.
					max_retries: 2,
					retry_backoff_initial: 500,
					retry_backoff_max: 5000,
					...(baseURL.trim() ? { base_url: baseURL.trim() } : {}),
					...(allowPrivate ? { allow_private_network: true } : {})
				}
			};
			if (selected === 'custom') {
				body.custom_provider_config = { base_provider_type: customBase, is_key_less: keyless };
				if (customBase === 'openai' && !responsesAPI) {
					// The gateway then serves Responses and Messages requests through Chat Completions.
					body.custom_provider_config.allowed_requests = CHAT_COMPLETIONS_ONLY_REQUESTS;
				}
			}
			await createProvider(body, { dontLogErrors: true });
			created = true;

			const hasKeyValues = fields.some((f) => values[f.path]?.trim());
			if (hasKeyValues) {
				const key: Record<string, unknown> = {
					name: keyName(),
					models: ['*'],
					weight: 1,
					enabled: true
				};
				if (!fields.some((f) => f.path === 'value')) key.value = '';
				for (const f of fields) {
					const v = values[f.path]?.trim();
					if (v) setPath(key, f.path, v);
				}
				// eslint-disable-next-line @typescript-eslint/no-explicit-any
				await createProviderKey(providerID, key as any, { dontLogErrors: true });
			}
			dialog?.close();
			onCreated?.(providerID);
		} catch (e) {
			const msg = e instanceof Error ? e.message : String(e);
			error = /"message":"([^"]+)"/.exec(msg)?.[1] ?? msg;
			// Don't leave a half-configured provider behind if its key was rejected.
			if (created) {
				await deleteProvider(providerID, { dontLogErrors: true }).catch(() => {});
			}
		} finally {
			saving = false;
		}
	}
</script>

<ResponsiveDialog
	bind:this={dialog}
	title={selected
		? m.models_providers_add_named({
				name: selected === 'custom' ? m.models_providers_custom_provider_title() : selected.label
			})
		: m.models_providers_add_provider()}
	class="max-w-3xl"
>
	{#if !selected}
		<div class="flex flex-col gap-4 pt-4">
			<input
				class="text-input-filled"
				placeholder={m.models_providers_search_providers()}
				bind:value={search}
			/>
			<div
				class="grid max-h-[60vh] grid-cols-1 gap-2 overflow-y-auto sm:grid-cols-2 md:grid-cols-3"
			>
				{#each catalog as p (p.id)}
					<button
						class="bg-base-200 hover:bg-base-300 dark:bg-base-100 dark:hover:bg-base-200 flex items-start gap-3 rounded-md p-3 text-left transition-colors"
						onclick={() => {
							selected = p;
							reset();
						}}
					>
						{#if p.icon}
							<img
								src={p.icon}
								alt=""
								class="size-7 shrink-0 rounded object-contain {p.monochromeIcon
									? 'dark:invert'
									: ''}"
							/>
						{:else}
							<div
								class="bg-base-100 flex size-7 shrink-0 items-center justify-center rounded text-xs font-bold uppercase"
							>
								{p.label.slice(0, 2)}
							</div>
						{/if}
						<div class="flex flex-col">
							<span class="text-sm font-medium">{p.label}</span>
							<span class="text-muted-content line-clamp-2 text-xs">{p.description}</span>
						</div>
					</button>
				{/each}
				<button
					class="bg-base-200 hover:bg-base-300 dark:bg-base-100 dark:hover:bg-base-200 flex items-start gap-3 rounded-md p-3 text-left transition-colors"
					onclick={() => {
						selected = 'custom';
						reset();
					}}
				>
					<Blocks class="size-7 shrink-0" />
					<div class="flex flex-col">
						<span class="text-sm font-medium">{m.models_providers_custom_provider()}</span>
						<span class="text-muted-content text-xs"
							>{m.models_providers_custom_provider_description()}</span
						>
					</div>
				</button>
			</div>
		</div>
	{:else}
		<form
			class="flex flex-col gap-4 p-4 md:p-0"
			onsubmit={(e) => {
				e.preventDefault();
				save();
			}}
		>
			{#if !direct}
				<button
					type="button"
					class="text-muted-content flex w-fit items-center gap-1 text-xs"
					onclick={() => (selected = undefined)}
				>
					<ChevronLeft class="size-3" />
					{m.models_providers_all_providers()}
				</button>
			{/if}

			{#if selected === 'custom'}
				<div class="grid grid-cols-1 gap-4 md:grid-cols-2">
					<div class="flex flex-col gap-1">
						<label for="custom-name" class="text-sm font-light">{m.core_name()}</label>
						<input
							id="custom-name"
							class="text-input-filled"
							bind:value={customName}
							placeholder="my-llm"
						/>
						<span class="text-muted-content text-xs"
							>{m.models_providers_custom_name_help_prefix()}
							<code>{customName || 'name'}/model</code
							>{m.models_providers_custom_name_help_suffix()}</span
						>
					</div>
					<div class="flex flex-col gap-1">
						<label for="custom-base" class="text-sm font-light"
							>{m.models_providers_api_format()}</label
						>
						<select id="custom-base" class="select" bind:value={customBase}>
							{#each CUSTOM_PROVIDER_BASES as b (b.id)}
								<option value={b.id}>{b.label}</option>
							{/each}
						</select>
					</div>
				</div>
				<label class="flex items-center gap-2 text-sm">
					<input type="checkbox" class="checkbox checkbox-sm" bind:checked={keyless} />
					{m.models_providers_no_api_key_required()}
				</label>
				{#if customBase === 'openai'}
					<div class="flex flex-col gap-1">
						<label class="flex items-center gap-2 text-sm">
							<input
								type="checkbox"
								class="checkbox checkbox-sm"
								bind:checked={responsesAPI}
								aria-describedby="new-responses-api-help"
							/>
							{m.models_providers_responses_api()}
						</label>
						<span id="new-responses-api-help" class="text-muted-content text-xs">
							{m.models_providers_responses_api_help()}
						</span>
					</div>
				{/if}
			{/if}

			{#each fields as field (field.path)}
				<div class="flex flex-col gap-1">
					<label for={`new-${field.path}`} class="text-sm font-light">
						{field.label}
						{#if 'required' in field && field.required}<span class="text-error">*</span>{/if}
					</label>
					{#if field.sensitive}
						<SensitiveInput
							name={`new-${field.path}`}
							bind:value={() => values[field.path] ?? '', (v) => (values[field.path] = v)}
							textarea={'multiline' in field && !!field.multiline}
							placeholder={'placeholder' in field ? field.placeholder : undefined}
						/>
					{:else}
						<input
							id={`new-${field.path}`}
							class="text-input-filled"
							placeholder={'placeholder' in field ? field.placeholder : undefined}
							bind:value={() => values[field.path] ?? '', (v) => (values[field.path] = v)}
						/>
					{/if}
					{#if 'description' in field && field.description}
						<span class="text-muted-content text-xs">{field.description}</span>
					{/if}
				</div>
			{/each}

			<div class="flex flex-col gap-1">
				<label for="new-base-url" class="text-sm font-light">
					{m.models_providers_base_url()}
					{#if selected === 'custom'}<span class="text-error">*</span>{:else}<span
							class="text-muted-content">{m.models_providers_optional_suffix()}</span
						>{/if}
				</label>
				<input
					id="new-base-url"
					class="text-input-filled"
					bind:value={baseURL}
					placeholder={selected === 'custom'
						? 'https://llm.internal.example.com/v1'
						: m.models_providers_base_url_default_placeholder()}
				/>
			</div>
			<div class="flex flex-col gap-1">
				<label class="flex items-center gap-2 text-sm">
					<input
						type="checkbox"
						class="checkbox checkbox-sm"
						bind:checked={allowPrivate}
						aria-describedby="new-allow-private-help"
					/>
					{m.models_providers_allow_private_network()}
				</label>
				<span id="new-allow-private-help" class="text-muted-content text-xs">
					{m.models_providers_allow_private_network_help()}
				</span>
			</div>

			{#if error}
				<div class="notification-error rounded-md p-2 text-sm">{error}</div>
			{/if}

			<div class="flex justify-end gap-2">
				<button type="button" class="btn btn-secondary text-sm" onclick={() => dialog?.close()}>
					{m.common_cancel()}
				</button>
				<button type="submit" class="btn btn-primary text-sm" disabled={!valid || saving}>
					{#if saving}<Loading class="size-4" />{:else}{m.models_providers_add_provider()}{/if}
				</button>
			</div>
		</form>
	{/if}
</ResponsiveDialog>

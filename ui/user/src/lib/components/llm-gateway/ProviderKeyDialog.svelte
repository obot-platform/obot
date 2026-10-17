<script lang="ts">
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import SensitiveInput from '$lib/components/SensitiveInput.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		createProviderKey,
		updateProviderKey,
		type ProviderKey
	} from '$lib/services/llm-gateway/gateway';
	import {
		catalogEntry,
		getPath,
		setPath,
		type KeyField
	} from '$lib/services/llm-gateway/providers';

	interface Props {
		provider: string;
		// Base provider type for custom providers; decides which fields apply.
		baseProvider?: string;
		existingKeyNames?: string[];
		onSaved?: (key: ProviderKey) => void;
	}

	let { provider, baseProvider, existingKeyNames = [], onSaved }: Props = $props();

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let editing = $state<ProviderKey>();
	let values = $state<Record<string, string>>({});
	let name = $state('');
	let models = $state('*');
	let blacklisted = $state('');
	let weight = $state(1);
	let enabled = $state(true);
	let saving = $state(false);
	let error = $state('');

	let fields = $derived<KeyField[]>(
		catalogEntry(baseProvider ?? provider)?.keyFields ?? [
			{ path: 'value', label: m.models_providers_api_key(), sensitive: true, required: true }
		]
	);

	function defaultName() {
		for (let i = 1; ; i++) {
			const candidate = `${provider}-key-${i}`;
			if (!existingKeyNames.includes(candidate)) return candidate;
		}
	}

	export function open(key?: ProviderKey) {
		editing = key;
		error = '';
		values = {};
		if (key) {
			name = key.name;
			models = (key.models ?? []).join(', ');
			blacklisted = (key.blacklisted_models ?? []).join(', ');
			weight = key.weight ?? 1;
			enabled = key.enabled ?? true;
			for (const f of fields) {
				const v = getPath(key as unknown as Record<string, unknown>, f.path);
				if (v && typeof v === 'object' && 'value' in (v as Record<string, unknown>)) {
					values[f.path] = String((v as Record<string, unknown>).value ?? '');
				} else if (typeof v === 'string') {
					values[f.path] = v;
				}
			}
		} else {
			name = defaultName();
			models = '*';
			blacklisted = '';
			weight = 1;
			enabled = true;
		}
		dialog?.open();
	}

	function split(v: string) {
		return v
			.split(',')
			.map((s) => s.trim())
			.filter(Boolean);
	}

	let valid = $derived(
		name.trim().length > 0 &&
			fields.every((f) => !f.required || !!values[f.path]?.trim() || (!!editing && f.sensitive))
	);

	async function save() {
		saving = true;
		error = '';
		try {
			const body: Record<string, unknown> = {
				name: name.trim(),
				models: split(models).length ? split(models) : ['*'],
				blacklisted_models: split(blacklisted),
				weight: Number(weight) || 1,
				enabled
			};
			if (!fields.some((f) => f.path === 'value')) body.value = '';
			for (const f of fields) {
				const v = values[f.path]?.trim() ?? '';
				if (!v && !f.required && f.path !== 'value') continue;
				setPath(body, f.path, v);
			}
			const key = body as unknown as ProviderKey;
			const saved = editing?.id
				? await updateProviderKey(provider, editing.id, key, { dontLogErrors: true })
				: await createProviderKey(provider, key, { dontLogErrors: true });
			onSaved?.(saved);
			dialog?.close();
		} catch (e) {
			error = errorMessage(e);
		} finally {
			saving = false;
		}
	}

	function errorMessage(e: unknown): string {
		const msg = e instanceof Error ? e.message : String(e);
		const match = /"message":"([^"]+)"/.exec(msg);
		return match?.[1] ?? msg;
	}
</script>

<ResponsiveDialog
	bind:this={dialog}
	title={editing ? m.models_providers_edit_key_title() : m.models_providers_add_key()}
	class="max-w-xl"
>
	<form
		class="flex flex-col gap-4 pt-4"
		onsubmit={(e) => {
			e.preventDefault();
			save();
		}}
	>
		<div class="flex flex-col gap-1">
			<label for="key-name" class="text-sm font-light">{m.core_name()}</label>
			<input id="key-name" class="text-input-filled" bind:value={name} />
			<span class="text-muted-content text-xs">{m.models_providers_key_name_help()}</span>
		</div>

		{#each fields as field (field.path)}
			<div class="flex flex-col gap-1">
				<label for={`key-${field.path}`} class="text-sm font-light">
					{field.label}
					{#if field.required}<span class="text-error">*</span>{/if}
				</label>
				{#if field.sensitive}
					<SensitiveInput
						name={`key-${field.path}`}
						bind:value={() => values[field.path] ?? '', (v) => (values[field.path] = v)}
						textarea={field.multiline}
						placeholder={editing
							? m.models_providers_key_keep_current_placeholder()
							: field.placeholder}
					/>
				{:else}
					<input
						id={`key-${field.path}`}
						class="text-input-filled"
						placeholder={field.placeholder}
						bind:value={() => values[field.path] ?? '', (v) => (values[field.path] = v)}
					/>
				{/if}
				{#if field.description}
					<span class="text-muted-content text-xs">{field.description}</span>
				{/if}
			</div>
		{/each}

		<div class="grid grid-cols-1 gap-4 md:grid-cols-2">
			<div class="flex flex-col gap-1">
				<label for="key-models" class="text-sm font-light"
					>{m.models_providers_key_allowed_models()}</label
				>
				<input
					id="key-models"
					class="text-input-filled"
					bind:value={models}
					placeholder={m.models_providers_key_allowed_models_placeholder()}
				/>
				<span class="text-muted-content text-xs"
					>{m.models_providers_key_comma_separated()} <code>*</code>
					{m.models_providers_key_star_allows_all()}</span
				>
			</div>
			<div class="flex flex-col gap-1">
				<label for="key-blacklist" class="text-sm font-light"
					>{m.models_providers_key_blocked_models()}</label
				>
				<input
					id="key-blacklist"
					class="text-input-filled"
					bind:value={blacklisted}
					placeholder={m.models_providers_optional()}
				/>
			</div>
			<div class="flex flex-col gap-1">
				<label for="key-weight" class="text-sm font-light">{m.models_providers_key_weight()}</label>
				<input
					id="key-weight"
					type="number"
					min="0"
					step="0.1"
					class="text-input-filled"
					bind:value={weight}
				/>
				<span class="text-muted-content text-xs">{m.models_providers_key_weight_help()}</span>
			</div>
			<div class="flex items-center pt-6">
				<Toggle
					label={m.core_status_enabled()}
					labelInline
					checked={enabled}
					onChange={(v) => (enabled = v)}
				/>
			</div>
		</div>

		{#if error}
			<div class="notification-error rounded-md p-2 text-sm">{error}</div>
		{/if}

		<div class="flex justify-end gap-2">
			<button type="button" class="btn btn-secondary text-sm" onclick={() => dialog?.close()}>
				{m.common_cancel()}
			</button>
			<button type="submit" class="btn btn-primary text-sm" disabled={!valid || saving}>
				{#if saving}<Loading class="size-4" />{:else}{m.core_save()}{/if}
			</button>
		</div>
	</form>
</ResponsiveDialog>

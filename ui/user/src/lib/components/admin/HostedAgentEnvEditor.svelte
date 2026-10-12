<script lang="ts">
	import { m } from '$lib/i18n';
	import type { HostedAgentEnv } from '$lib/services/admin/types';
	import IconButton from '../primitives/IconButton.svelte';
	import { Eye, EyeOff, Plus, Trash2 } from '@lucide/svelte';

	interface Props {
		env: HostedAgentEnv[];
		readonly?: boolean;
		onReveal?: () => Promise<Record<string, string>>;
	}

	let { env = $bindable([]), readonly, onReveal }: Props = $props();

	// Keys that look like credentials default to sensitive.
	const SENSITIVE_KEY_RE = /PASS|TOKEN|KEY/i;

	// Tracks which rows the user has toggled by hand, so the heuristic never
	// overrides an explicit choice. Local only: never sent to the API.
	let touchedSensitive = $state<Set<number>>(new Set());
	let revealed = $state(false);
	let revealing = $state(false);

	function onKeyInput(index: number, key: string) {
		env[index].key = key;
		if (!touchedSensitive.has(index)) {
			env[index].sensitive = SENSITIVE_KEY_RE.test(key);
		}
	}

	function toggleSensitive(index: number) {
		touchedSensitive.add(index);
		touchedSensitive = touchedSensitive;
		env[index].sensitive = !env[index].sensitive;
	}

	function addRow() {
		env = [
			...env,
			{ key: '', value: '', name: '', description: '', sensitive: false, required: false }
		];
	}

	function removeRow(index: number) {
		env = env.filter((_, i) => i !== index);
		touchedSensitive.delete(index);
		touchedSensitive = touchedSensitive;
	}

	async function reveal() {
		if (!onReveal) return;
		revealing = true;
		try {
			const secrets = await onReveal();
			for (const item of env) {
				if (item.sensitive && secrets[item.key] !== undefined) {
					item.value = secrets[item.key];
				}
			}
			revealed = true;
		} finally {
			revealing = false;
		}
	}
</script>

<div class="flex flex-col gap-2">
	<div class="mb-2 flex items-center justify-between">
		<div class="flex flex-col">
			<h2 class="text-lg font-semibold">{m.hosted_agents_templates_environment_title()}</h2>
			<span class="text-muted-content text-xs">
				{m.hosted_agents_templates_environment_hint()}
			</span>
		</div>
		<div class="flex items-center gap-2">
			{#if onReveal && env.some((e) => e.sensitive) && !readonly}
				<button
					class="btn btn-secondary flex items-center gap-1 text-sm"
					disabled={revealing || revealed}
					onclick={reveal}
				>
					{#if revealed}
						<EyeOff class="size-4" /> {m.hosted_agents_templates_environment_revealed()}
					{:else}
						<Eye class="size-4" /> {m.core_reveal()}
					{/if}
				</button>
			{/if}
			{#if !readonly}
				<button class="btn btn-primary flex items-center gap-1 text-sm" onclick={addRow}>
					<Plus class="size-4" />
					{m.hosted_agents_templates_environment_add_variable()}
				</button>
			{/if}
		</div>
	</div>

	{#if env.length === 0}
		<p class="text-muted-content py-4 text-center text-sm">
			{m.hosted_agents_templates_environment_empty()}
		</p>
	{:else}
		<div class="flex flex-col gap-3">
			{#each env as item, i (i)}
				<div
					class="dark:bg-base-400 dark:border-base-400 bg-base-100 flex flex-col gap-3 rounded-lg border border-transparent p-4"
				>
					<div class="flex items-end gap-3">
						<div class="flex flex-1 flex-col gap-2">
							<label for="env-key-{i}" class="text-sm font-light">{m.hosted_agents_key()}</label>
							<input
								id="env-key-{i}"
								value={item.key}
								oninput={(e) => onKeyInput(i, e.currentTarget.value)}
								class="text-input-filled"
								placeholder="API_TOKEN"
								disabled={readonly}
							/>
						</div>
						<div class="flex flex-1 flex-col gap-2">
							<label for="env-value-{i}" class="text-sm font-light">{m.core_col_value()}</label>
							<input
								id="env-value-{i}"
								bind:value={item.value}
								type={item.sensitive && !revealed ? 'password' : 'text'}
								class="text-input-filled"
								placeholder={item.sensitive
									? m.hosted_agents_templates_environment_stored_securely()
									: ''}
								disabled={readonly}
							/>
						</div>
						{#if !readonly}
							<IconButton
								variant="danger"
								onclick={() => removeRow(i)}
								tooltip={{ text: m.core_remove() }}
							>
								<Trash2 class="size-4" />
							</IconButton>
						{/if}
					</div>

					<div class="flex items-end gap-3">
						<div class="flex flex-1 flex-col gap-2">
							<label for="env-desc-{i}" class="text-sm font-light">{m.core_description()}</label>
							<input
								id="env-desc-{i}"
								bind:value={item.description}
								class="text-input-filled"
								disabled={readonly}
							/>
						</div>
						<div class="flex items-center gap-4 pb-2">
							<label class="flex items-center gap-2 text-sm font-light">
								<input
									type="checkbox"
									class="checkbox checkbox-sm"
									checked={item.sensitive}
									onchange={() => toggleSensitive(i)}
									disabled={readonly}
								/>
								{m.hosted_agents_sensitive()}
							</label>
							<label class="flex items-center gap-2 text-sm font-light">
								<input
									type="checkbox"
									class="checkbox checkbox-sm"
									bind:checked={item.required}
									disabled={readonly}
								/>
								{m.hosted_agents_required()}
							</label>
						</div>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>

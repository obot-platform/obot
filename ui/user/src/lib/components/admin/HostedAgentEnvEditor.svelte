<script lang="ts">
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
			<h2 class="text-lg font-semibold">Environment</h2>
			<span class="text-muted-content text-xs">
				Values marked sensitive are stored separately and are not shown after saving.
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
						<EyeOff class="size-4" /> Revealed
					{:else}
						<Eye class="size-4" /> Reveal
					{/if}
				</button>
			{/if}
			{#if !readonly}
				<button class="btn btn-primary flex items-center gap-1 text-sm" onclick={addRow}>
					<Plus class="size-4" /> Add Variable
				</button>
			{/if}
		</div>
	</div>

	{#if env.length === 0}
		<p class="text-muted-content py-4 text-center text-sm">No environment variables added.</p>
	{:else}
		<div class="flex flex-col gap-3">
			{#each env as item, i (i)}
				<div
					class="dark:bg-base-400 dark:border-base-400 bg-base-100 flex flex-col gap-3 rounded-lg border border-transparent p-4"
				>
					<div class="flex items-end gap-3">
						<div class="flex flex-1 flex-col gap-2">
							<label for="env-key-{i}" class="text-sm font-light">Key</label>
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
							<label for="env-value-{i}" class="text-sm font-light">Value</label>
							<input
								id="env-value-{i}"
								bind:value={item.value}
								type={item.sensitive && !revealed ? 'password' : 'text'}
								class="text-input-filled"
								placeholder={item.sensitive ? 'Stored securely' : ''}
								disabled={readonly}
							/>
						</div>
						{#if !readonly}
							<IconButton
								variant="danger"
								onclick={() => removeRow(i)}
								tooltip={{ text: 'Remove' }}
							>
								<Trash2 class="size-4" />
							</IconButton>
						{/if}
					</div>

					<div class="flex items-end gap-3">
						<div class="flex flex-1 flex-col gap-2">
							<label for="env-desc-{i}" class="text-sm font-light">Description</label>
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
								Sensitive
							</label>
							<label class="flex items-center gap-2 text-sm font-light">
								<input
									type="checkbox"
									class="checkbox checkbox-sm"
									bind:checked={item.required}
									disabled={readonly}
								/>
								Required
							</label>
						</div>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>

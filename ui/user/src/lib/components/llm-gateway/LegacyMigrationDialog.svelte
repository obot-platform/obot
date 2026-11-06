<script lang="ts">
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import { m } from '$lib/i18n';
	import {
		getMigration,
		pendingMigrationFamily,
		resolveMigration,
		type MigrationStatus
	} from '$lib/services/llm-gateway/gateway';
	import { profile } from '$lib/stores';
	import { ChevronRight } from '@lucide/svelte';
	import { onMount } from 'svelte';

	let status = $state<MigrationStatus>();
	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let selection = $state('');
	let saving = $state(false);
	let error = $state('');
	let opened = false;
	let active = true;
	let refreshVersion = 0;
	const deferralKey = 'obot-legacy-migration-deferred';
	let canManage = $derived(
		!!profile.current.hasAdminAccess?.() && !profile.current.isAdminReadonly?.()
	);
	let family = $derived(pendingMigrationFamily(status));
	let options = $derived(
		Object.entries(status?.sources ?? {}).filter(([, s]) => s.provider === family)
	);

	async function refresh() {
		if (!canManage || saving) return;
		const version = ++refreshVersion;
		try {
			const next = await getMigration({ dontLogErrors: true });
			if (!active || version !== refreshVersion) return;
			status = next;
			if (family && !opened && sessionStorage.getItem(deferralKey) !== 'true') {
				opened = true;
				dialog?.open();
			}
		} catch {
			// A periodic refresh retries transient startup failures. Explicit saves show errors.
		}
	}
	function defer() {
		sessionStorage.setItem(deferralKey, 'true');
		dialog?.close();
	}
	async function save() {
		if (!status || !family || !selection) return;
		saving = true;
		refreshVersion++;
		error = '';
		try {
			status = await resolveMigration(family, selection, status.version);
			selection = '';
			if (!family) dialog?.close();
			window.dispatchEvent(new Event('llm-migration-updated'));
		} catch (cause) {
			error = cause instanceof Error ? cause.message : m.models_migration_failed();
			saving = false;
			await refresh();
			selection = '';
		} finally {
			saving = false;
		}
	}
	onMount(() => {
		active = true;
		void refresh();
		const timer = window.setInterval(() => void refresh(), 15000);
		const reopen = () => {
			if (!canManage) return;
			error = '';
			selection = '';
			dialog?.open();
			void refresh();
		};
		window.addEventListener('open-llm-migration', reopen);
		return () => {
			active = false;
			clearInterval(timer);
			window.removeEventListener('open-llm-migration', reopen);
		};
	});
</script>

{#if canManage}
	<ResponsiveDialog
		bind:this={dialog}
		title={m.models_migration_title()}
		hideClose
		disableClickOutside
		disableEscape
	>
		<div class="space-y-4">
			<p>{m.models_migration_description()}</p>
			{#if family}
				<fieldset class="space-y-3" disabled={saving}>
					<legend class="font-semibold">{family === 'azure' ? 'Azure' : 'Amazon Bedrock'}</legend>
					{#each options as [id, source] (id)}
						<div class="border-base-300 rounded-lg border p-3">
							<label class="flex cursor-pointer gap-3">
								<input
									type="radio"
									name="legacy-provider"
									value={id}
									bind:group={selection}
									class="radio mt-1"
								/>
								<span class="min-w-0 break-words">
									<span class="block font-medium">{source.name ?? id}</span>
									{#if source.endpoint}<span class="text-base-content/70 block text-sm"
											>{source.endpoint}</span
										>{/if}
									{#if source.region}<span class="text-base-content/70 block text-sm"
											>{source.region}</span
										>{/if}
								</span>
							</label>
							{#if source.models.length}
								<!-- Inspecting models must not select the provider's radio button. -->
								<details class="group border-base-300 mt-3 border-t pt-3">
									<summary
										class="text-base-content/70 flex cursor-pointer list-none items-center gap-1.5 text-sm font-medium [&::-webkit-details-marker]:hidden"
									>
										<ChevronRight
											class="size-4 shrink-0 transition-transform group-open:rotate-90"
										/>
										{m.models_migration_models({ count: source.models.length })}
									</summary>
									<ul
										class="bg-base-200/50 border-base-300 divide-base-300 mt-2 max-h-48 divide-y overflow-y-auto rounded-md border text-sm"
									>
										{#each source.models as model, index (index)}
											<li class="px-3 py-2"><code class="block break-all">{model}</code></li>
										{/each}
									</ul>
								</details>
							{/if}
						</div>
					{/each}
					<label class="border-base-300 flex cursor-pointer gap-3 rounded-lg border p-3">
						<input
							type="radio"
							name="legacy-provider"
							value="both"
							bind:group={selection}
							class="radio mt-1"
						/>
						<span>
							<span class="block font-medium">{m.models_migration_shared()}</span>
							<span class="text-base-content/70 block text-sm"
								>{m.models_migration_shared_help()}</span
							>
						</span>
					</label>
				</fieldset>
			{:else}
				<p>{m.models_migration_no_pending()}</p>
			{/if}
			{#if error}<p role="alert" class="text-error">{error}</p>{/if}
			<div class="flex justify-end gap-2">
				<button class="btn btn-ghost" onclick={defer} disabled={saving}
					>{m.models_migration_later()}</button
				>
				{#if family}<button class="btn btn-primary" onclick={save} disabled={!selection || saving}
						>{saving ? m.models_migration_saving() : m.models_migration_apply()}</button
					>{/if}
			</div>
		</div>
	</ResponsiveDialog>
{/if}

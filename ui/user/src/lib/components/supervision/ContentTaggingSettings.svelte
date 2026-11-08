<script lang="ts">
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import {
		flushContentTagging,
		getContentTaggingStatus,
		getProvenanceSettings,
		updateProvenanceSettings,
		type ContentTagging,
		type ContentTaggingMode,
		type ContentTaggingStatus,
		type ProvenanceSettings
	} from '$lib/services/provenance';
	import { profile } from '$lib/stores';
	import { success } from '$lib/stores/success';
	import { formatTimeAgo } from '$lib/time';
	import { ScanText } from '@lucide/svelte';

	// Settings for the optional AI content tagger: real time or batched, and how much of the
	// eligible content it samples.

	let settings = $state<ProvenanceSettings>();
	let tagging = $state<ContentTagging>({ enabled: false, onlyUnlabeled: true });
	let samplePercent = $state(100);
	let status = $state<ContentTaggingStatus>();
	let error = $state('');
	let busy = $state(false);
	let canEdit = $derived(!!profile.current.isAdmin?.());

	const modes: { value: ContentTaggingMode; label: () => string }[] = [
		{ value: 'async', label: m.supervision_tagging_mode_async },
		{ value: 'sync', label: m.supervision_tagging_mode_sync },
		{ value: 'batch', label: m.supervision_tagging_mode_batch }
	];

	async function load() {
		try {
			const loaded = await getProvenanceSettings({ dontLogErrors: true });
			settings = loaded;
			tagging = {
				...{ enabled: false, onlyUnlabeled: true, mode: 'async', batchIntervalMinutes: 15 },
				...loaded.contentTagging
			};
			const rate = tagging.sampleRate ?? 0;
			samplePercent = rate > 0 ? Math.round(rate * 100) : 100;
			status = await getContentTaggingStatus({ dontLogErrors: true });
		} catch (e) {
			error = parseErrorContent(e).message;
		}
	}

	$effect(() => {
		load();
	});

	async function save() {
		if (!settings) return;
		busy = true;
		error = '';
		try {
			const percent = Math.min(100, Math.max(1, Math.round(samplePercent || 100)));
			const contentTagging: ContentTagging = {
				...tagging,
				sampleRate: percent >= 100 ? 0 : percent / 100
			};
			settings = await updateProvenanceSettings({ ...settings, contentTagging });
			success.add(m.supervision_tagging_saved());
		} catch (e) {
			error = parseErrorContent(e).message;
		} finally {
			busy = false;
		}
	}

	async function runNow() {
		busy = true;
		error = '';
		try {
			status = await flushContentTagging();
		} catch (e) {
			error = parseErrorContent(e).message;
		} finally {
			busy = false;
		}
	}
</script>

<section
	class="dark:bg-base-200 bg-base-100 flex flex-col gap-4 rounded-lg p-4 shadow-sm"
	aria-labelledby="content-tagging-title"
>
	<div class="flex items-start gap-3">
		<ScanText class="text-primary mt-0.5 size-5 shrink-0" />
		<div class="flex flex-col gap-1">
			<h2 id="content-tagging-title" class="font-semibold">{m.supervision_tagging_title()}</h2>
			<p class="text-muted-content text-sm">{m.supervision_tagging_description()}</p>
		</div>
	</div>

	{#if error}
		<div class="notification-error rounded-md p-3 text-sm" role="alert">{error}</div>
	{/if}

	<fieldset class="flex flex-col gap-3" disabled={!canEdit || busy}>
		<label class="flex items-center gap-2 text-sm">
			<input type="checkbox" bind:checked={tagging.enabled} />
			{m.supervision_tagging_enabled()}
		</label>

		{#if tagging.enabled}
			<div class="ml-6 flex flex-col gap-3">
				<div
					class="flex flex-col gap-2"
					role="radiogroup"
					aria-label={m.supervision_tagging_mode()}
				>
					<span class="text-sm font-medium">{m.supervision_tagging_mode()}</span>
					{#each modes as mode (mode.value)}
						<label class="flex items-center gap-2 text-sm">
							<input type="radio" value={mode.value} bind:group={tagging.mode} />
							{mode.label()}
						</label>
					{/each}
				</div>
				<div class="flex flex-wrap gap-4">
					<label class="flex flex-col gap-1 text-sm">
						{m.supervision_tagging_sample_rate()}
						<input
							class="text-input-filled w-32"
							type="number"
							min="1"
							max="100"
							bind:value={samplePercent}
						/>
					</label>
					{#if tagging.mode === 'batch'}
						<label class="flex flex-col gap-1 text-sm">
							{m.supervision_tagging_interval()}
							<input
								class="text-input-filled w-32"
								type="number"
								min="1"
								max="1440"
								bind:value={tagging.batchIntervalMinutes}
							/>
						</label>
					{/if}
				</div>
				<label class="flex items-center gap-2 text-sm">
					<input type="checkbox" bind:checked={tagging.onlyUnlabeled} />
					{m.supervision_tagging_only_unlabeled()}
				</label>
				{#if tagging.mode === 'batch' && status}
					<div class="text-muted-content flex flex-wrap items-center gap-3 text-sm" role="status">
						<span>{m.supervision_tagging_queue({ queued: status.queued })}</span>
						{#if status.lastRun}
							<span>
								{m.supervision_tagging_last_run({
									time: formatTimeAgo(status.lastRun).relativeTime,
									tagged: status.lastTagged
								})}
							</span>
						{/if}
						<button type="button" class="btn btn-secondary text-sm" onclick={runNow}>
							{m.supervision_tagging_run_now()}
						</button>
					</div>
				{/if}
			</div>
		{/if}
	</fieldset>

	{#if canEdit}
		<div class="flex justify-end">
			<button type="button" class="btn btn-primary" disabled={busy || !settings} onclick={save}>
				{m.supervision_tagging_save()}
			</button>
		</div>
	{/if}
</section>

<script lang="ts">
	import { MCP_FILTERS_FIELD_IDS } from '$lib/constants';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';

	interface Props {
		exists: boolean;
		disabled: boolean;
		dirty: boolean;
		busy?: boolean;
		blocked?: boolean;
		readonly?: boolean;
		activationFailed?: boolean;
		canReapply?: boolean;
		error?: string;
		onSave: (disabled: boolean) => void;
		onDisable: () => void;
		onCancel: () => void;
	}
	let {
		exists,
		disabled,
		dirty,
		busy,
		blocked,
		readonly,
		activationFailed,
		canReapply,
		error,
		onSave,
		onDisable,
		onCancel
	}: Props = $props();
</script>

<div
	class="bg-base-200 dark:bg-base-100 sticky bottom-0 z-50 flex w-full flex-wrap items-center justify-between gap-3 py-4"
>
	<div role="status" aria-live="polite" class="flex flex-col gap-1 text-sm">
		<div class="flex items-center gap-2">
			<span
				class={activationFailed
					? 'text-error font-medium'
					: exists && !disabled
						? 'pill-primary bg-primary'
						: 'text-muted-content'}
			>
				{activationFailed
					? m.mcps_filters_activation_failed()
					: exists && !disabled
						? m.core_status_enabled()
						: m.core_status_disabled()}
			</span>
			<span aria-hidden="true" class="text-muted-content">·</span>
			<span
				class={dirty || !exists
					? 'text-amber-700 dark:text-amber-400 font-medium'
					: 'text-muted-content'}
				>{dirty || !exists ? m.mcps_filters_unsaved_changes() : m.mcps_filters_saved()}</span
			>
		</div>
		{#if dirty && exists && !disabled}
			<p class="text-muted-content text-xs">{m.mcps_filters_edits_not_active()}</p>
		{/if}
	</div>
	{#if !readonly}
		<div class="flex flex-wrap items-center gap-2">
			{#if exists && !disabled}
				<button class="btn btn-soft btn-error text-sm" disabled={busy} onclick={onDisable}
					>{m.mcps_filters_disable()}</button
				>
			{/if}
			<button class="btn btn-secondary" disabled={busy} onclick={onCancel}
				>{m.common_cancel()}</button
			>
			{#if !exists || disabled}
				<button
					class="btn btn-secondary text-sm"
					disabled={busy || blocked || (exists && !dirty && !activationFailed && !canReapply)}
					onclick={() => onSave(true)}>{m.mcps_filters_save_disabled()}</button
				>
			{/if}
			<button
				id={MCP_FILTERS_FIELD_IDS.saveBtn}
				class="btn btn-primary text-sm"
				disabled={busy ||
					blocked ||
					(exists && !disabled && !dirty && !activationFailed && !canReapply)}
				onclick={() => onSave(false)}
			>
				{#if busy}<Loading class="size-4" />{:else if !exists || disabled}
					{exists && !dirty ? m.mcps_filters_enable() : m.mcps_filters_save_enable()}
				{:else}{m.mcps_filters_save_apply()}{/if}
			</button>
		</div>
	{/if}
	{#if error}<p role="alert" class="text-error w-full text-sm">{error}</p>{/if}
</div>

<script lang="ts">
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		MAX_TOOL_PREFIX_LENGTH,
		TOOL_NAME_CHARSET_REGEX,
		TOOL_NAME_SPECIAL_CHAR_WARNING,
		type ToolNameIssue
	} from '$lib/services/user/mcp';

	interface Props {
		readonly?: boolean;
		otherNames?: string[];
		otherToolPrefixes?: string[];
		onSave?: (details: { name: string; toolPrefix: string }) => void | Promise<void>;
		onClose?: () => void;
	}

	let {
		readonly = false,
		otherNames = [],
		otherToolPrefixes = [],
		onSave,
		onClose
	}: Props = $props();

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let name = $state('');
	let toolPrefix = $state('');
	let saving = $state(false);

	let nameError = $derived.by(() => {
		const trimmed = name.trim();
		if (!trimmed) return m.vmcps_component_name_required();
		if (otherNames.some((existing) => existing.trim() === trimmed)) {
			return m.vmcps_component_name_taken({ name: trimmed });
		}
	});

	let prefixIssue = $derived.by((): ToolNameIssue | undefined => {
		const prefix = toolPrefix ?? '';
		if (!TOOL_NAME_CHARSET_REGEX.test(prefix)) {
			return { severity: 'error', message: m.mcps_composite_prefix_invalid() };
		}
		if (prefix.length > MAX_TOOL_PREFIX_LENGTH) {
			return {
				severity: 'error',
				message: m.mcps_composite_prefix_too_long({ count: MAX_TOOL_PREFIX_LENGTH })
			};
		}
		const trimmed = prefix.trim();
		if (trimmed && otherToolPrefixes.some((existing) => existing === trimmed)) {
			return {
				severity: 'error',
				message: m.mcps_composite_prefix_duplicate({ prefix: trimmed })
			};
		}
		if (/[./]/.test(prefix)) {
			return { severity: 'warning', message: TOOL_NAME_SPECIAL_CHAR_WARNING };
		}
	});

	let saveDisabled = $derived(saving || Boolean(nameError) || prefixIssue?.severity === 'error');

	export function open(details: { name: string; toolPrefix?: string }) {
		name = details.name;
		toolPrefix = details.toolPrefix ?? '';
		saving = false;
		dialog?.open();
	}

	export function close() {
		dialog?.close();
	}

	async function handleSave() {
		if (readonly || saveDisabled) return;
		saving = true;
		try {
			await onSave?.({ name: name.trim(), toolPrefix: toolPrefix.trim() });
		} catch {
			saving = false;
		}
	}
</script>

<ResponsiveDialog
	bind:this={dialog}
	class="max-w-sm"
	title={readonly ? m.vmcps_view_details() : m.vmcps_edit_details()}
	{onClose}
>
	<form
		class="flex flex-col gap-4 px-4 md:px-0"
		onsubmit={(event) => {
			event.preventDefault();
			void handleSave();
		}}
	>
		<label class="flex flex-col gap-1">
			<span class="font-light text-sm">{m.vmcps_component_name()} *</span>
			<input
				class="text-input-filled"
				bind:value={name}
				autocomplete="off"
				required
				readonly={readonly || saving}
			/>
		</label>
		{#if !readonly && nameError}
			<p class="text-error -mt-2 text-sm" role="alert">{nameError}</p>
		{/if}

		<div class="flex flex-col gap-1">
			<div class="flex items-center justify-between gap-1.5">
				<label class="text-sm font-light" for="vmcp-component-prefix">
					{m.mcps_composite_tool_name_prefix()}
				</label>
				{#if !readonly}
					<button
						type="button"
						class="btn btn-secondary btn-xs"
						disabled={saving}
						onclick={() => {
							toolPrefix = '';
						}}
					>
						{m.core_clear()}
					</button>
				{/if}
			</div>
			<div class="flex items-center gap-2">
				<input
					id="vmcp-component-prefix"
					class="text-input-filled flex-1"
					placeholder={m.mcps_composite_no_prefix()}
					bind:value={toolPrefix}
					autocomplete="off"
					readonly={readonly || saving}
				/>
			</div>
			{#if prefixIssue}
				<p class={`text-xs ${prefixIssue.severity === 'error' ? 'text-error' : 'text-warning'}`}>
					{prefixIssue.message}
				</p>
			{:else}
				<p class="text-muted-content text-[11px]">
					{m.mcps_composite_prefix_hint()}
				</p>
			{/if}
		</div>

		<div class="flex md:flex-row flex-col justify-end gap-2 mt-4">
			<button
				type="button"
				class="btn btn-ghost rounded-full"
				onclick={() => dialog?.close()}
				disabled={saving}
			>
				{readonly ? m.core_close() : m.common_cancel()}
			</button>
			{#if !readonly}
				<button type="submit" class="btn btn-primary" disabled={saveDisabled}>
					{#if saving}
						<Loading class="text-primary-content size-4" />
					{:else}
						{m.core_save()}
					{/if}
				</button>
			{/if}
		</div>
	</form>
</ResponsiveDialog>

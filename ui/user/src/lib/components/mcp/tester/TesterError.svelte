<script lang="ts">
	import JsonPreview from '$lib/components/JsonPreview.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import { m } from '$lib/i18n';
	import CornerCopyButton from './CornerCopyButton.svelte';
	import type { Snippet } from 'svelte';

	interface Props {
		title: string;
		message: string;
		details?: unknown;
		rawResponse?: unknown;
		actions?: Snippet;
		icon?: Snippet;
		severity?: 'error' | 'warning';
	}
	let { title, message, details, rawResponse, actions, icon, severity = 'error' }: Props = $props();
	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let open = $state(false);
	let rawOpen = $state(false);
	let trigger: HTMLButtonElement;
	let errorText = $derived.by(() => {
		const value = details ?? message;
		try {
			return JSON.stringify(typeof value === 'string' ? JSON.parse(value) : value, null, 2);
		} catch {
			return String(value);
		}
	});
	let preview = $derived(message.length > 400 ? `${message.slice(0, 400)}…` : message);
</script>

<section
	class={`${severity === 'warning' ? 'notification-alert' : 'notification-error'} mb-4 min-w-0 shrink-0 rounded-lg p-4`}
	role={severity === 'warning' ? 'status' : 'alert'}
>
	<div class="flex items-center gap-2">
		{#if icon}{@render icon()}{/if}
		<h3 class="font-medium">{title}</h3>
	</div>
	<p class="mt-1 line-clamp-3 text-sm whitespace-pre-wrap wrap-break-word">{preview}</p>
	<div class="mt-3 flex flex-wrap gap-2">
		<button
			bind:this={trigger}
			type="button"
			class="btn btn-secondary btn-sm"
			onclick={() => dialog?.open()}>{m.mcps_tester_view_full_error()}</button
		>
		{#if actions}{@render actions()}{/if}
	</div>
</section>

<ResponsiveDialog
	bind:this={dialog}
	title={m.mcps_tester_error_details()}
	closeButtonLabel={m.common_close()}
	class="max-h-[90dvh] max-w-4xl overflow-hidden"
	classes={{ content: 'min-h-0 max-h-[90dvh]', header: 'shrink-0' }}
	onOpen={() => (open = true)}
	onClose={() => {
		open = false;
		rawOpen = false;
		trigger?.focus();
	}}
>
	{#if open}
		<div class="default-scrollbar-thin mt-4 min-h-0 min-w-0 space-y-4 overflow-y-auto">
			<CornerCopyButton text={errorText} label={m.mcps_tester_copy_error()}>
				<JsonPreview
					value={details ?? message}
					maxHeight="65dvh"
					ariaLabel={m.mcps_tester_full_error()}
				/>
			</CornerCopyButton>
			{#if rawResponse !== undefined}
				<details bind:open={rawOpen}>
					<summary class="cursor-pointer text-sm font-medium"
						>{m.mcps_tester_raw_response()}</summary
					>
					{#if rawOpen}
						<CornerCopyButton
							text={JSON.stringify(rawResponse, null, 2)}
							label={m.mcps_tester_copy_raw_json()}
							class="mt-2"
						>
							<JsonPreview
								value={rawResponse}
								maxHeight="45dvh"
								ariaLabel={m.mcps_tester_raw_mcp_response()}
							/>
						</CornerCopyButton>
					{/if}
				</details>
			{/if}
		</div>
	{/if}
</ResponsiveDialog>

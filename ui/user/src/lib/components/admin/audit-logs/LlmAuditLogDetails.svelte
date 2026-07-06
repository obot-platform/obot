<script lang="ts">
	import { isAbortError } from '$lib/errors';
	import { AdminService, type LLMAuditLog } from '$lib/services';
	import { parseJSON } from '$lib/services/nanobot/utils';
	import AuditLogDetails from './AuditLogDetails.svelte';
	import { CircleAlert } from '@lucide/svelte';

	export type LlmAuditLogDetail = LLMAuditLog & { user: string };
	interface Props {
		id: string;
		auditLog: LlmAuditLogDetail;
		onClose: () => void;
	}
	let { id, auditLog, onClose }: Props = $props();
	let loading = $state(true);
	let fetchError = $state<string | null>(null);
	let additData = $state<LLMAuditLog>();

	$effect(() => {
		if (!id) return;

		const controller = new AbortController();
		loading = true;
		fetchError = null;

		AdminService.getLLMAuditLog(id, { signal: controller.signal })
			.then((response) => {
				if (controller.signal.aborted) return;
				additData = {
					...response,
					responseHeaders: parseJSON(response.responseHeaders) ?? undefined,
					requestHeaders: parseJSON(response.requestHeaders) ?? undefined,
					requestBody:
						parseJSON(response.requestBody) ?? parseJSON(response.redactedRequestBody) ?? undefined,
					responseBody: parseJSON(response.responseBody) ?? undefined
				};
			})
			.catch((err) => {
				if (isAbortError(err) || controller.signal.aborted) return;
				console.error('Failed to fetch LLM audit log details:', err);
				fetchError = err instanceof Error ? err.message : 'Failed to load audit log details';
			})
			.finally(() => {
				if (controller.signal.aborted) return;
				loading = false;
			});

		return () => controller.abort();
	});

	const titles = {
		modelProvider: 'Model Provider',
		modelID: 'Model ID'
	};

	const properties = ['modelProvider', 'modelID'] as const;
</script>

{#if fetchError}
	<div class="notification-error m-4 flex items-center gap-3 p-3">
		<CircleAlert class="size-4 shrink-0" />
		<div class="flex flex-col gap-1">
			<p class="text-sm font-semibold">Unable to load full audit log details</p>
			<p class="text-sm font-light">{fetchError}</p>
		</div>
	</div>
{/if}

<AuditLogDetails
	auditLog={{
		...auditLog,
		...additData
	}}
	{onClose}
	loading={{
		request: loading,
		response: loading
	}}
	shouldShowPayload
>
	{#snippet preRequestContent(data)}
		<div class="flex flex-wrap gap-2 p-4 pl-5">
			{#each properties as property (property)}
				{#if data[property as keyof typeof data]}
					<div class="bg-base-400 rounded-full px-3 py-1 text-[11px] font-light truncate">
						<span class="font-medium">{titles[property as keyof typeof titles]}:</span>
						{data[property as keyof typeof data]}
					</div>
				{/if}
			{/each}
		</div>
	{/snippet}
</AuditLogDetails>

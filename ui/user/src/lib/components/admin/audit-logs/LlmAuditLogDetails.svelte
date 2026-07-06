<script lang="ts">
	import { AdminService, type LLMAuditLog } from '$lib/services';
	import AuditLogDetail from './AuditLogDetail.svelte';

	export type LlmAuditLogDetail = LLMAuditLog & { user: string };
	interface Props {
		id: string;
		auditLog: LlmAuditLogDetail;
		onClose: () => void;
	}
	let { id, auditLog, onClose }: Props = $props();
	let loading = $state(true);
	let additData = $state<LLMAuditLog>();

	$effect(() => {
		if (id) {
			loading = true;
			AdminService.getLLMAuditLog(id)
				.then((response) => {
					additData = {
						...response,
						responseHeaders: response.responseHeaders
							? JSON.parse(response.responseHeaders)
							: undefined,
						requestHeaders: response.requestHeaders
							? JSON.parse(response.requestHeaders)
							: undefined,
						requestBody: response.requestBody ? JSON.parse(response.requestBody) : undefined,
						redactedRequestBody: response.redactedRequestBody
							? JSON.parse(response.redactedRequestBody)
							: undefined,
						responseBody: response.responseBody ? JSON.parse(response.responseBody) : undefined
					};
				})
				.finally(() => {
					loading = false;
				});
		}
	});

	const titles = {
		modelProvider: 'Model Provider',
		modelID: 'Model ID'
	};

	const properties = ['modelProvider', 'modelID'] as const;
</script>

<AuditLogDetail
	auditLog={{
		...auditLog,
		...additData
	}}
	{onClose}
	loading={{
		request: loading,
		response: loading
	}}
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
</AuditLogDetail>

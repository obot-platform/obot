<script lang="ts">
	import { m } from '$lib/i18n';
	import {
		describeAction,
		toolLabelReviewURL,
		type Approval,
		type ToolLabelReviewRequest
	} from '$lib/services/provenance';
	import { resolve } from '$lib/url';

	let {
		approvals,
		labelReviews = [],
		policyReviews = []
	}: {
		approvals: Approval[];
		labelReviews?: ToolLabelReviewRequest[];
		policyReviews?: { id: string; name: string; reason: string }[];
	} = $props();
	let showAllLabels = $state(false);
	const labelLimit = 3;
	let shownLabels = $derived(showAllLabels ? labelReviews : labelReviews.slice(0, labelLimit));
</script>

<ul class="divide-base-300 divide-y">
	{#each shownLabels as review (`${review.resourceType}:${review.resourceID}`)}
		<li class="flex items-center gap-4 py-3 first:pt-0 last:pb-0">
			<div class="flex min-w-0 flex-1 flex-col gap-1">
				<p class="truncate text-sm font-medium">{review.displayName}</p>
				<p class="text-muted-content text-xs">
					{m.supervision_attention_label_tools({
						type: review.resourceType === 'vmcp' ? 'vMCP' : 'MCP',
						count: review.toolCount
					})}
				</p>
			</div>
			<a href={toolLabelReviewURL(review)} class="btn btn-secondary btn-sm shrink-0">
				{m.supervision_attention_review_labels()}
			</a>
		</li>
	{/each}
	{#if labelReviews.length > labelLimit}
		<li class="py-2">
			<button
				type="button"
				class="btn btn-ghost btn-sm text-xs"
				onclick={() => (showAllLabels = !showAllLabels)}
			>
				{showAllLabels
					? m.supervision_insights_show_fewer()
					: m.supervision_attention_all_label_reviews({ count: labelReviews.length })}
			</button>
		</li>
	{/if}
	{#each policyReviews as policy (policy.id)}
		<li class="flex items-center gap-4 py-3 first:pt-0 last:pb-0">
			<div class="flex min-w-0 flex-1 flex-col gap-1">
				<p class="truncate text-sm font-medium">{policy.name}</p>
				<p class="text-muted-content text-xs">{m.supervision_attention_policy_suggestion()}</p>
				{#if policy.reason}<p class="text-muted-content line-clamp-2 text-xs">
						{policy.reason}
					</p>{/if}
			</div>
			<a
				href={resolve(`/policies/${encodeURIComponent(policy.id)}`)}
				class="btn btn-secondary btn-sm shrink-0"
			>
				{m.supervision_attention_review_policy()}
			</a>
		</li>
	{/each}
	{#each approvals as approval (approval.id)}
		<li class="flex items-center gap-4 py-3 first:pt-0 last:pb-0">
			<div class="flex min-w-0 flex-1 flex-col gap-1">
				<p class="truncate text-sm font-medium" title={describeAction(approval.action)}>
					{describeAction(approval.action)}
				</p>
				<p class="text-muted-content truncate text-xs">{approval.policyName}</p>
				{#if approval.triage}
					<p class="text-muted-content text-xs">
						{approval.triage.recommendation === 'approve'
							? m.supervision_recommend_approve()
							: approval.triage.recommendation === 'deny'
								? m.supervision_recommend_deny()
								: m.supervision_recommend_escalate()}
					</p>
				{/if}
			</div>
			<a
				href={resolve(`/approvals?approval=${encodeURIComponent(approval.id)}`)}
				class="btn btn-secondary btn-sm shrink-0"
			>
				{m.supervision_attention_review()}
			</a>
		</li>
	{/each}
</ul>

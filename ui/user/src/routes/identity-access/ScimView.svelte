<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { parseErrorContent } from '$lib/errors';
	import { AdminService } from '$lib/services';
	import type {
		GroupReference,
		SCIMConnection,
		SCIMConnectionReview,
		SCIMGroupList,
		SCIMPage,
		SCIMRequestFailure,
		SCIMSetupGroup,
		SCIMSetupUser,
		SCIMSetupWarning
	} from '$lib/services/admin/types';
	import { profile } from '$lib/stores';
	import { adminConfigStore } from '$lib/stores/adminConfig.svelte';
	import { formatTimeAgo } from '$lib/time';
	import {
		ChevronLeft,
		ChevronRight,
		Circle,
		CircleAlert,
		CircleCheck,
		Info,
		TriangleAlert
	} from '@lucide/svelte';
	import { untrack } from 'svelte';
	import { fade } from 'svelte/transition';

	interface Props {
		// The review of the SCIM connection, or undefined when there is none.
		review?: SCIMConnectionReview;
		// Each list's page size, which the review's first pages were loaded with.
		pageSize?: number;
	}

	interface IssuedToken {
		title: string;
		baseURL: string;
		token: string;
	}

	type PagedList =
		| 'provisionedUsers'
		| 'unprovisionedUsers'
		| 'boundGroups'
		| 'unboundReferencedGroups'
		| 'unreferencedGroups'
		| 'failures';

	type TokenAction = 'generate' | 'rotate' | 'revokePrevious' | 'revokeCurrent';

	type Paged<T> = SCIMPage<T> & { offset: number };

	interface Pages {
		provisionedUsers: Paged<SCIMSetupUser>;
		unprovisionedUsers: Paged<SCIMSetupUser>;
		boundGroups: Paged<SCIMSetupGroup>;
		unboundReferencedGroups: Paged<SCIMSetupGroup>;
		unreferencedGroups: Paged<SCIMSetupGroup>;
		failures: Paged<SCIMRequestFailure>;
	}

	const referenceLabels: Record<GroupReference['kind'], string> = {
		accessControlRule: 'access control rule',
		modelAccessPolicy: 'model access policy',
		skillAccessRule: 'skill access rule',
		messagePolicy: 'message policy',
		hostedAgentAccessRule: 'hosted agent access rule',
		publishedArtifact: 'published artifact',
		groupRoleAssignment: 'group role assignment',
		vmcpProfile: 'virtual MCP server'
	};

	// Names each list in its pager, so the pagers of different lists are told apart.
	const listNouns: Record<PagedList, string> = {
		provisionedUsers: 'provisioned users',
		unprovisionedUsers: 'unprovisioned users',
		boundGroups: 'pushed groups',
		unboundReferencedGroups: 'referenced groups not pushed yet',
		unreferencedGroups: 'unreferenced groups',
		failures: 'recent failures'
	};

	const groupLists: Record<
		'boundGroups' | 'unboundReferencedGroups' | 'unreferencedGroups',
		SCIMGroupList
	> = {
		boundGroups: 'bound',
		unboundReferencedGroups: 'unboundReferenced',
		unreferencedGroups: 'unreferenced'
	};

	let { review: initialReview, pageSize = 50 }: Props = $props();
	let review = $state(untrack(() => initialReview));
	let connection = $derived(review?.connection);
	let providerName = $derived(connection?.authProviderDisplayName || 'the identity provider');

	// The page each list shows, which starts as the review's first page.
	let pages = $state<Pages>(untrack(() => pagesFrom(initialReview)));

	let isOwner = $derived(!!profile.current.isOwner?.());
	let isBootstrapUser = $derived(!!profile.current.isBootstrapUser?.());
	// Owners, including the bootstrap user, manage the token, so the identity provider can be set up
	// before any Owner has signed in through it. Only an Owner who signed in through it can enforce.
	let canManageToken = $derived(isOwner);
	let canEnforce = $derived(isOwner && !isBootstrapUser);

	let loading = $state(false);
	let actionError = $state<string>();
	let notice = $state<string>();
	let confirmEnforce = $state(false);
	let confirmTokenAction = $state<TokenAction>();

	// Errors show inline, next to what failed, rather than also as a notification.
	const quiet = { dontLogErrors: true };
	// Counts loads of the review, and each list's page requests, so late responses can be dropped.
	let reviewGeneration = 0;
	const pageRequests: Record<PagedList, number> = {
		provisionedUsers: 0,
		unprovisionedUsers: 0,
		boundGroups: 0,
		unboundReferencedGroups: 0,
		unreferencedGroups: 0,
		failures: 0
	};

	let issuedToken = $state<IssuedToken>();
	let tokenDialog = $state<ReturnType<typeof ResponsiveDialog>>();

	let checklist = $derived.by(() => {
		if (!review || !connection) return [];
		return [
			{
				title: 'Generate the token',
				detail: 'Obot shows the bearer token once, when it is issued.',
				done: connection.hasToken
			},
			{
				title: `Create the SCIM app in ${providerName}`,
				detail: `Enter the base URL and the bearer token in a SCIM 2.0 application. Done once ${providerName} sends a request.`,
				done: !!review.activity.lastRequestAt
			},
			{
				title: 'Assign users',
				detail: `${review.provisionedUsers.total} provisioned, ${review.unprovisionedUsers.total} not provisioned yet.`,
				done: review.provisionedUsers.total > 0
			},
			{
				title: 'Push groups',
				detail: `${review.boundGroups.total} pushed. Roles and policies can then be granted to them.`,
				done: review.boundGroups.total > 0
			},
			{
				title: 'Enforce',
				detail: 'Only provisioned users can sign in afterwards.',
				done: connection.state === 'enforced'
			}
		];
	});

	function firstPage<T>(page?: SCIMPage<T>): Paged<T> {
		return { items: page?.items ?? [], total: page?.total ?? 0, offset: 0 };
	}

	function pagesFrom(r?: SCIMConnectionReview): Pages {
		return {
			provisionedUsers: firstPage(r?.provisionedUsers),
			unprovisionedUsers: firstPage(r?.unprovisionedUsers),
			boundGroups: firstPage(r?.boundGroups),
			unboundReferencedGroups: firstPage(r?.unboundReferencedGroups),
			unreferencedGroups: firstPage(r?.unreferencedGroups),
			failures: firstPage(r?.activity.recentFailures)
		};
	}

	function describeReference(ref: GroupReference) {
		const label = referenceLabels[ref.kind] ?? ref.kind;
		if (ref.kind === 'groupRoleAssignment') {
			return ref.detail ? `${label} (${ref.detail})` : label;
		}
		let description = ref.displayName ? `${label} “${ref.displayName}”` : `${label} ${ref.id}`;
		if (ref.detail) {
			description += `, ${ref.detail}`;
		}
		return description;
	}

	function timeAgo(timestamp?: string) {
		return formatTimeAgo(timestamp).relativeTime || 'Never';
	}

	function showToken(title: string, conn: SCIMConnection) {
		if (!conn.token) return;
		issuedToken = { title, baseURL: conn.baseURL, token: conn.token };
		tokenDialog?.open();
	}

	async function refresh() {
		if (!connection) return;
		reviewGeneration++;
		review = await AdminService.getSCIMConnectionReview(connection.id, {
			limit: pageSize,
			...quiet
		});
		pages = pagesFrom(review);
	}

	async function showPage(list: PagedList, offset: number) {
		if (!connection) return;
		const page = { offset: Math.max(offset, 0), limit: pageSize };
		const request = ++pageRequests[list];
		const generation = reviewGeneration;
		// A response for a page that has since been replaced, by a newer page or by loading the review
		// again, is dropped.
		const current = () => request === pageRequests[list] && generation === reviewGeneration;
		// The list shrank since it was shown, so the page is past its end: show its last page instead.
		const pastEnd = (result: SCIMPage<unknown>) =>
			result.items.length === 0 && result.total > 0 && page.offset > 0;
		const lastPage = (total: number) => Math.floor((total - 1) / pageSize) * pageSize;

		actionError = undefined;
		try {
			if (list === 'provisionedUsers' || list === 'unprovisionedUsers') {
				const result = await AdminService.listSCIMUsers(
					connection.id,
					list === 'provisionedUsers',
					page,
					quiet
				);
				if (!current()) return;
				if (pastEnd(result)) return showPage(list, lastPage(result.total));
				pages[list] = { ...result, offset: page.offset };
			} else if (list === 'failures') {
				const result = await AdminService.listSCIMFailures(connection.id, page, quiet);
				if (!current()) return;
				if (pastEnd(result)) return showPage(list, lastPage(result.total));
				pages.failures = { ...result, offset: page.offset };
			} else {
				const result = await AdminService.listSCIMGroups(
					connection.id,
					groupLists[list],
					page,
					quiet
				);
				if (!current()) return;
				if (pastEnd(result)) return showPage(list, lastPage(result.total));
				pages[list] = { ...result, offset: page.offset };
			}
		} catch (err) {
			if (current()) actionError = parseErrorContent(err).message;
		}
	}

	async function handleEnforce() {
		if (!connection) return;
		loading = true;
		actionError = undefined;
		notice = undefined;
		try {
			const result = await AdminService.enforceSCIM(connection.id);
			notice = `SCIM is enforced. ${result.disabledUserCount} unprovisioned users were disabled, and ${result.deletedGroupCount} unreferenced groups were deleted.`;
			await refresh();
			// The layout stops asking Owners to finish setting up SCIM.
			void adminConfigStore.refresh();
		} catch (err) {
			actionError = parseErrorContent(err).message;
			// Whatever refused Enforce may have changed what the review shows, such as its blockers.
			try {
				await refresh();
			} catch {
				// The error above explains what happened.
			}
		} finally {
			confirmEnforce = false;
			loading = false;
		}
	}

	async function handleTokenAction(action: TokenAction) {
		if (!connection) return;
		loading = true;
		actionError = undefined;
		try {
			switch (action) {
				case 'generate':
					showToken('SCIM token', await AdminService.rotateSCIMToken(connection.id, quiet));
					break;
				case 'rotate':
					showToken('New SCIM token', await AdminService.rotateSCIMToken(connection.id, quiet));
					break;
				case 'revokeCurrent':
					showToken(
						'Replacement SCIM token',
						await AdminService.revokeCurrentSCIMToken(connection.id, quiet)
					);
					break;
				case 'revokePrevious':
					await AdminService.revokePreviousSCIMToken(connection.id, quiet);
					break;
			}
			await refresh();
		} catch (err) {
			actionError = parseErrorContent(err).message;
		} finally {
			confirmTokenAction = undefined;
			loading = false;
		}
	}

	const tokenConfirmations: Record<
		TokenAction,
		{ title: string; msg: string; note: string; submit: string; type: 'info' | 'delete' }
	> = {
		generate: {
			title: 'Generate token',
			msg: 'Issue the SCIM bearer token?',
			note: 'The token is shown only once. Copy it into the SCIM application of the identity provider.',
			submit: 'Generate token',
			type: 'info'
		},
		rotate: {
			title: 'Rotate token',
			msg: 'Issue a new SCIM bearer token?',
			note: 'The current token keeps working for a day, or until you revoke it, so you can update the identity provider without failed requests.',
			submit: 'Rotate token',
			type: 'info'
		},
		revokePrevious: {
			title: 'Revoke previous token',
			msg: 'Stop accepting the previous SCIM token?',
			note: 'Requests that still use it fail. Make sure the identity provider uses the new token first.',
			submit: 'Revoke',
			type: 'delete'
		},
		revokeCurrent: {
			title: 'Revoke current token',
			msg: 'Replace the current SCIM token?',
			note: 'Use this when the token has leaked. A new token is issued, and both the current and the previous token stop working at once, so provisioning fails until the identity provider uses the new one.',
			submit: 'Revoke and replace',
			type: 'delete'
		}
	};
</script>

<div class="flex flex-col gap-6 pb-8" in:fade={{ duration: PAGE_TRANSITION_DURATION }}>
	{#if actionError}
		<div class="notification-error flex items-start gap-2" role="alert">
			<CircleAlert class="mt-0.5 size-5 shrink-0 text-error" />
			<p class="text-sm font-light whitespace-pre-line wrap-break-word">{actionError}</p>
		</div>
	{/if}
	{#if notice}
		<div class="notification-info flex items-start gap-2" role="status">
			<CircleCheck class="mt-0.5 size-5 shrink-0" />
			<p class="text-sm font-light">{notice}</p>
		</div>
	{/if}

	{#if connection && review}
		{@render connectionDetails(connection)}
		{#if connection.origin === 'scim_first' && connection.state === 'connected'}
			{@render setupChecklist()}
		{/if}
		{#if connection.state === 'connected'}
			{@render enforceSection(review)}
		{/if}
		{@render groupsSection(review)}
		{@render usersSection(connection)}
		{@render activitySection(review)}
	{:else}
		<section class="paper" aria-labelledby="scim-none-title">
			<h2 id="scim-none-title" class="text-lg font-semibold">SCIM provisioning is not set up</h2>
			<p class="text-muted-content text-sm font-light">
				To provision users and groups through SCIM, configure an auth provider that supports it,
				such as Okta, on the Auth Providers tab, and leave its directory credentials (the API
				Services client ID and private key) empty. Setup then continues here once an Owner has
				signed in.
			</p>
			<p class="text-muted-content text-sm font-light">
				An auth provider that already fetches groups from its directory at sign-in cannot be moved
				to SCIM here yet.
			</p>
		</section>
	{/if}
</div>

{#snippet connectionDetails(conn: SCIMConnection)}
	<section class="paper" aria-labelledby="scim-connection-title">
		<div class="flex flex-wrap items-center gap-2">
			<h2 id="scim-connection-title" class="text-lg font-semibold">SCIM provisioning</h2>
			<span class={conn.state === 'enforced' ? 'pill-primary' : 'pill-warning'}>
				{conn.state === 'enforced' ? 'Enforced' : 'Connected'}
			</span>
		</div>
		<p class="text-muted-content text-sm font-light">
			{#if conn.state === 'enforced'}
				Signing in with {providerName} requires an account that {providerName} provisioned.
			{:else}
				{providerName} provisions users and groups through SCIM. Users it has not provisioned can still
				sign in until SCIM is enforced.
			{/if}
		</p>

		{#if !conn.authProviderConfigured}
			<div class="notification-alert flex items-start gap-2 text-sm font-light" role="alert">
				<TriangleAlert class="mt-0.5 size-5 shrink-0" />
				<span>
					{providerName} is not the configured auth provider, so SCIM requests fail. Once it is configured
					again, retry the failed provisioning tasks in {providerName}.
				</span>
			</div>
		{/if}

		<dl class="grid grid-cols-1 gap-4 text-sm md:grid-cols-2">
			<div class="flex min-w-0 flex-col gap-1 md:col-span-2">
				<dt class="text-muted-content text-xs">Base URL</dt>
				<dd class="min-w-0 break-all">
					<CopyButton showTextLeft buttonText={conn.baseURL} text={conn.baseURL} />
				</dd>
			</div>
			<div class="flex flex-col gap-1">
				<dt class="text-muted-content text-xs">Auth provider</dt>
				<dd>{providerName}</dd>
			</div>
			<div class="flex flex-col gap-1">
				<dt class="text-muted-content text-xs">Bearer token</dt>
				<dd>
					{#if conn.hasToken}
						Issued {timeAgo(conn.tokenIssuedAt)}
					{:else}
						Not generated yet
					{/if}
				</dd>
			</div>
			{#if conn.previousTokenAccepted}
				<div class="flex flex-col gap-1">
					<dt class="text-muted-content text-xs">Previous token</dt>
					<dd>
						Accepted until {new Date(conn.previousTokenExpiresAt ?? '').toLocaleString()}
					</dd>
				</div>
			{/if}
			<div class="flex flex-col gap-1">
				<dt class="text-muted-content text-xs">Enforced</dt>
				<dd>{conn.enforcedAt ? timeAgo(conn.enforcedAt) : 'Not yet'}</dd>
			</div>
		</dl>

		{#if canManageToken}
			<div class="flex flex-wrap justify-end gap-2">
				{#if conn.hasToken}
					{#if conn.previousTokenAccepted}
						<button
							class="btn btn-secondary"
							disabled={loading}
							onclick={() => (confirmTokenAction = 'revokePrevious')}
						>
							Revoke previous token
						</button>
					{/if}
					<button
						class="btn btn-secondary"
						disabled={loading}
						onclick={() => (confirmTokenAction = 'revokeCurrent')}
					>
						Revoke current token
					</button>
					<button
						class="btn btn-secondary"
						disabled={loading}
						onclick={() => (confirmTokenAction = 'rotate')}
					>
						Rotate token
					</button>
				{:else}
					<button
						class="btn btn-primary"
						disabled={loading}
						onclick={() => (confirmTokenAction = 'generate')}
					>
						Generate token
					</button>
				{/if}
			</div>
		{/if}
	</section>
{/snippet}

{#snippet setupChecklist()}
	<section class="paper" aria-labelledby="scim-setup-title">
		<div class="flex flex-col gap-1">
			<h2 id="scim-setup-title" class="text-lg font-semibold">Set up provisioning</h2>
			<p class="text-muted-content text-sm font-light">
				Until SCIM is enforced, anyone assigned to the {providerName} sign-in app can sign in, and gets
				an account with no groups.
			</p>
		</div>
		<ol class="flex flex-col gap-3">
			{#each checklist as item, index (item.title)}
				<li class="flex items-start gap-3">
					{#if item.done}
						<CircleCheck class="text-success mt-0.5 size-5 shrink-0" aria-label="Done" />
					{:else}
						<Circle class="text-muted-content mt-0.5 size-5 shrink-0" aria-label="Not done" />
					{/if}
					<div class="flex flex-col">
						<span class="text-sm font-medium">{index + 1}. {item.title}</span>
						<span class="text-muted-content text-xs font-light">{item.detail}</span>
					</div>
				</li>
			{/each}
		</ol>
	</section>
{/snippet}

{#snippet enforceSection(r: SCIMConnectionReview)}
	<section class="paper" aria-labelledby="scim-enforce-title">
		<div class="flex flex-col gap-1">
			<h2 id="scim-enforce-title" class="text-lg font-semibold">Enforce SCIM</h2>
			<p class="text-muted-content text-sm font-light">
				Enforcing requires an account that {providerName} provisioned to sign in, disables the {r
					.unprovisionedUsers.total} users of {providerName} that it has not provisioned, and deletes
				the
				{r.unreferencedGroups.total} unbound groups that nothing references. Group memberships do not
				change, and nothing is deleted from the users. It cannot be undone.
			</p>
		</div>

		{#if r.enforceBlockers.length > 0}
			<div class="notification-alert flex items-start gap-2" role="alert">
				<TriangleAlert class="mt-0.5 size-5 shrink-0" />
				<div class="flex min-w-0 flex-col gap-1">
					<p class="text-sm font-medium">SCIM cannot be enforced yet</p>
					<ul class="list-disc pl-4 text-sm font-light wrap-break-word">
						{#each r.enforceBlockers as blocker (blocker)}
							<li>{blocker}</li>
						{/each}
					</ul>
				</div>
			</div>
		{/if}

		<div class="flex items-center justify-end gap-2">
			<button
				class="btn btn-primary"
				disabled={!canEnforce || loading || r.enforceBlockers.length > 0}
				onclick={() => (confirmEnforce = true)}
			>
				Enforce SCIM
			</button>
		</div>
	</section>
{/snippet}

{#snippet groupsSection(r: SCIMConnectionReview)}
	<section class="paper" aria-labelledby="scim-groups-title">
		<h2 id="scim-groups-title" class="text-lg font-semibold">Groups</h2>
		{@render warnings(r.warnings)}

		{#if pages.unboundReferencedGroups.total > 0}
			<div class="flex flex-col gap-2">
				<h3 class="text-sm font-semibold">
					Referenced groups not pushed yet ({pages.unboundReferencedGroups.total})
				</h3>
				<p class="text-muted-content text-xs font-light">
					Push each group from {providerName}, renaming it there first if its name differs from the
					one shown here, or remove its references. They block enforcing SCIM.
				</p>
				{@render groupList('unboundReferencedGroups', '')}
			</div>
		{/if}

		<div class="flex flex-col gap-2">
			<h3 class="text-sm font-semibold">Pushed groups ({pages.boundGroups.total})</h3>
			<p class="text-muted-content text-xs font-light">
				{providerName} manages these groups and their members. Grant roles and policies to them.
			</p>
			{@render groupList('boundGroups', `No groups have been pushed from ${providerName} yet.`)}
		</div>

		{#if pages.unreferencedGroups.total > 0}
			<div class="flex flex-col gap-2">
				<h3 class="text-sm font-semibold">
					Unreferenced groups ({pages.unreferencedGroups.total})
				</h3>
				<p class="text-muted-content text-xs font-light">
					Nothing references these unbound groups, so they grant nothing.
					{#if r.connection.state === 'connected'}Enforcing SCIM deletes them.{/if}
				</p>
				{@render groupList('unreferencedGroups', '')}
			</div>
		{/if}
	</section>
{/snippet}

{#snippet usersSection(conn: SCIMConnection)}
	<section class="paper" aria-labelledby="scim-users-title">
		<h2 id="scim-users-title" class="text-lg font-semibold">Users</h2>

		<div class="flex flex-col gap-2">
			<h3 class="text-sm font-semibold">Provisioned ({pages.provisionedUsers.total})</h3>
			{@render userList('provisionedUsers', `${providerName} has not provisioned any users yet.`)}
		</div>

		<div class="flex flex-col gap-2">
			<h3 class="text-sm font-semibold">Not provisioned ({pages.unprovisionedUsers.total})</h3>
			<p class="text-muted-content text-xs font-light">
				{#if conn.state === 'enforced'}
					These users cannot sign in until {providerName} provisions them, which re-enables them with
					their account and data.
				{:else}
					Assign these users to the SCIM application in {providerName}. Enforcing SCIM disables the
					ones it has not provisioned; provisioning them later re-enables them with their account
					and data.
				{/if}
			</p>
			{@render userList(
				'unprovisionedUsers',
				`Every user of ${providerName} has been provisioned.`
			)}
		</div>
	</section>
{/snippet}

{#snippet activitySection(r: SCIMConnectionReview)}
	<section class="paper" aria-labelledby="scim-activity-title">
		<div class="flex flex-col gap-1">
			<h2 id="scim-activity-title" class="text-lg font-semibold">Activity</h2>
			<p class="text-muted-content text-xs font-light">
				Requests show activity, not that {providerName} and Obot are synchronized.
			</p>
		</div>
		<dl class="grid grid-cols-1 gap-4 text-sm md:grid-cols-2">
			<div class="flex flex-col gap-1">
				<dt class="text-muted-content text-xs">Last request</dt>
				<dd>{timeAgo(r.activity.lastRequestAt)}</dd>
			</div>
			<div class="flex flex-col gap-1">
				<dt class="text-muted-content text-xs">Last successful request</dt>
				<dd>{timeAgo(r.activity.lastSuccessAt)}</dd>
			</div>
		</dl>

		<div class="flex flex-col gap-2">
			<h3 class="text-sm font-semibold">Recent failures</h3>
			{#if pages.failures.total === 0}
				<p class="text-muted-content text-sm font-light">No recent failures.</p>
			{:else}
				<ul class="divide-base-300 flex flex-col divide-y">
					{#each pages.failures.items as failure, i (i)}
						<li class="flex flex-col gap-0.5 py-2 text-sm">
							<div class="flex flex-wrap items-center gap-2">
								<span class="font-mono text-xs">{failure.method} {failure.resource}</span>
								<span class="pill-warning"
									>{failure.status}{failure.scimType ? ` ${failure.scimType}` : ''}</span
								>
								<span class="text-muted-content text-xs">{timeAgo(failure.time)}</span>
							</div>
							{#if failure.detail}
								<p class="text-muted-content text-xs font-light break-all">{failure.detail}</p>
							{/if}
						</li>
					{/each}
				</ul>
				{@render pager('failures')}
			{/if}
		</div>
	</section>
{/snippet}

{#snippet warnings(items: SCIMSetupWarning[])}
	{#if items.length > 0}
		<div class="notification-alert flex items-start gap-2" role="note">
			<TriangleAlert class="mt-0.5 size-5 shrink-0" />
			<ul class="flex min-w-0 flex-col gap-1 text-sm font-light wrap-break-word">
				{#each items as warning (warning.type + warning.groupID)}
					<li>
						{warning.message}
						{#if warning.references?.length}
							<span class="text-muted-content text-xs">
								Referenced by {warning.references.map(describeReference).join('; ')}.
							</span>
						{/if}
					</li>
				{/each}
			</ul>
		</div>
	{/if}
{/snippet}

{#snippet groupList(
	list: 'boundGroups' | 'unboundReferencedGroups' | 'unreferencedGroups',
	empty: string
)}
	{@const groups = pages[list].items}
	{#if pages[list].total === 0}
		{#if empty}
			<p class="text-muted-content text-sm font-light">{empty}</p>
		{/if}
	{:else}
		<ul class="divide-base-300 flex flex-col divide-y">
			{#each groups as group (group.id)}
				<li class="flex flex-col gap-1 py-2">
					<div class="flex flex-wrap items-center gap-2">
						<span class="text-sm font-medium">{group.name || group.id}</span>
						<span class="text-muted-content font-mono text-xs break-all">{group.id}</span>
						{#if group.consoleURL}
							<a
								class="text-link text-xs"
								href={group.consoleURL}
								target="_blank"
								rel="external noopener noreferrer"
							>
								Open in {providerName}
							</a>
						{/if}
					</div>
					{#if group.references?.length}
						<p class="text-muted-content text-xs font-light">
							Referenced by {group.references.map(describeReference).join('; ')}
						</p>
					{/if}
				</li>
			{/each}
		</ul>
		{@render pager(list)}
	{/if}
{/snippet}

{#snippet userList(list: 'provisionedUsers' | 'unprovisionedUsers', empty: string)}
	{@const users = pages[list].items}
	{#if pages[list].total === 0}
		<p class="text-muted-content text-sm font-light">{empty}</p>
	{:else}
		<ul class="divide-base-300 flex flex-col divide-y">
			{#each users as user (user.id)}
				<li class="flex flex-wrap items-center gap-2 py-2 text-sm">
					<span class="font-medium">{user.displayName || user.email || user.username}</span>
					{#if user.email && user.email !== user.displayName}
						<span class="text-muted-content text-xs">{user.email}</span>
					{/if}
					{#if user.status === 'disabled'}
						<span class="pill-warning">Disabled</span>
					{:else if user.scimID && !user.active}
						<span class="pill-warning">Deactivated</span>
					{/if}
					{#if !user.scimID && !user.signedIn}
						<span class="text-muted-content text-xs">Never signed in</span>
					{/if}
				</li>
			{/each}
		</ul>
		{@render pager(list)}
	{/if}
{/snippet}

{#snippet pager(list: PagedList)}
	{@const current = pages[list]}
	{@const noun = listNouns[list]}
	{#if current.total > pageSize || current.offset > 0}
		<div class="flex items-center justify-end gap-2 text-xs">
			<span class="text-muted-content">
				{current.offset + 1}–{Math.min(current.offset + current.items.length, current.total)} of {current.total}
				{noun}
			</span>
			<IconButton
				class="btn-sm"
				aria-label="Previous page of {noun}"
				disabled={loading || current.offset === 0}
				onclick={() => showPage(list, current.offset - pageSize)}
			>
				<ChevronLeft class="size-4" />
			</IconButton>
			<IconButton
				class="btn-sm"
				aria-label="Next page of {noun}"
				disabled={loading || current.offset + pageSize >= current.total}
				onclick={() => showPage(list, current.offset + pageSize)}
			>
				<ChevronRight class="size-4" />
			</IconButton>
		</div>
	{/if}
{/snippet}

<Confirm
	show={confirmEnforce}
	title="Enforce SCIM"
	msg="Enforce SCIM for {providerName}?"
	note="This cannot be undone. {review?.unprovisionedUsers.total ??
		0} users that {providerName} has not provisioned are disabled, and only provisioned users can sign in with {providerName} afterwards."
	submitText="Enforce SCIM"
	{loading}
	onsuccess={handleEnforce}
	oncancel={() => (confirmEnforce = false)}
/>

<Confirm
	show={!!confirmTokenAction}
	type={confirmTokenAction ? tokenConfirmations[confirmTokenAction].type : 'info'}
	title={confirmTokenAction ? tokenConfirmations[confirmTokenAction].title : ''}
	msg={confirmTokenAction ? tokenConfirmations[confirmTokenAction].msg : ''}
	note={confirmTokenAction ? tokenConfirmations[confirmTokenAction].note : ''}
	submitText={confirmTokenAction ? tokenConfirmations[confirmTokenAction].submit : ''}
	{loading}
	onsuccess={() => confirmTokenAction && handleTokenAction(confirmTokenAction)}
	oncancel={() => (confirmTokenAction = undefined)}
/>

<ResponsiveDialog
	bind:this={tokenDialog}
	title={issuedToken?.title}
	class="md:max-w-xl"
	disableClickOutside
	onClose={() => (issuedToken = undefined)}
>
	{#if issuedToken}
		<div class="flex flex-col gap-4 pt-2">
			<div class="notification-alert flex items-start gap-2 text-sm font-light">
				<Info class="mt-0.5 size-5 shrink-0" />
				<span>
					Copy the token now. It is shown only once. Enter the base URL and the token in the SCIM
					application in {providerName}.
				</span>
			</div>
			<div class="flex min-w-0 flex-col gap-1">
				<span class="text-muted-content text-xs">Base URL</span>
				<CopyButton showTextLeft buttonText={issuedToken.baseURL} text={issuedToken.baseURL} />
			</div>
			<div class="flex min-w-0 flex-col gap-1">
				<span class="text-muted-content text-xs">Bearer token</span>
				<code class="bg-base-200 dark:bg-base-300 rounded-md px-3 py-2 font-mono text-xs break-all">
					{issuedToken.token}
				</code>
				<CopyButton text={issuedToken.token} buttonText="Copy token" />
			</div>
			<div class="flex justify-end">
				<button class="btn btn-primary" onclick={() => tokenDialog?.close()}>Done</button>
			</div>
		</div>
	{/if}
</ResponsiveDialog>

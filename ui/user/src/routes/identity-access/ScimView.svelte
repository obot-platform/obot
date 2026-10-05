<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import { describeGroupReference } from '$lib/components/admin/scim/groupReferences';
	import { tokenExpiryState } from '$lib/components/admin/scim/tokenExpiry';
	import Pagination from '$lib/components/table/Pagination.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { parseErrorContent } from '$lib/errors';
	import { AdminService } from '$lib/services';
	import type {
		SCIMConnection,
		SCIMConnectionReview,
		SCIMEnablePreview,
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
	import { Circle, CircleAlert, CircleCheck, RefreshCw, TriangleAlert } from '@lucide/svelte';
	import { tick, untrack } from 'svelte';
	import { fade } from 'svelte/transition';

	interface Props {
		// The review of the SCIM connection, or undefined when there is none.
		review?: SCIMConnectionReview;
		// Whether SCIM can be enabled for the configured auth provider, while there is no connection.
		enablePreview?: SCIMEnablePreview;
		// Each list's page size, which the first pages of the review were loaded with.
		pageSize?: number;
	}

	interface IssuedToken {
		title: string;
		baseURL: string;
		token: string;
	}

	type GroupPagedList = 'boundGroups' | 'unboundReferencedGroups' | 'unreferencedGroups';

	type PagedList = 'provisionedUsers' | 'unprovisionedUsers' | GroupPagedList | 'failures';

	type TokenAction = 'generate' | 'rotate' | 'revokePrevious' | 'revokeCurrent';

	type SetupStepID = 'token' | 'app' | 'users' | 'groups' | 'enforce';

	interface SetupStep {
		id: SetupStepID;
		// Names the step in the progress indicator.
		label: string;
		title: string;
		done: boolean;
	}

	type Paged<T> = SCIMPage<T> & { offset: number };

	interface Pages {
		provisionedUsers: Paged<SCIMSetupUser>;
		unprovisionedUsers: Paged<SCIMSetupUser>;
		boundGroups: Paged<SCIMSetupGroup>;
		unboundReferencedGroups: Paged<SCIMSetupGroup>;
		unreferencedGroups: Paged<SCIMSetupGroup>;
		failures: Paged<SCIMRequestFailure>;
	}

	// Names each list in its pager, so the pagers of different lists are told apart.
	const listNouns: Record<PagedList, string> = {
		provisionedUsers: 'provisioned users',
		unprovisionedUsers: 'unprovisioned users',
		boundGroups: 'pushed groups',
		unboundReferencedGroups: 'referenced groups not pushed yet',
		unreferencedGroups: 'unreferenced groups',
		failures: 'recent failures'
	};

	const groupLists = {
		boundGroups: 'bound',
		unboundReferencedGroups: 'unboundReferenced',
		unreferencedGroups: 'unreferenced'
	} as const satisfies Record<GroupPagedList, SCIMGroupList>;

	let {
		review: initialReview,
		enablePreview: initialEnablePreview,
		pageSize = 50
	}: Props = $props();
	let review = $state(untrack(() => initialReview));
	let enablePreview = $state(untrack(() => initialEnablePreview));
	let connection = $derived(review?.connection);
	// Enabling SCIM applies only to a configured auth provider that supports it.
	let offerEnable = $derived(!connection && !!enablePreview?.authProviderName);
	let providerName = $derived(
		connection?.authProviderDisplayName ||
			enablePreview?.authProviderDisplayName ||
			'the identity provider'
	);

	// The page each list shows, which starts as the review's first page.
	let pages = $state<Pages>(untrack(() => pagesFrom(initialReview)));

	let isOwner = $derived(!!profile.current.isOwner?.());
	let isBootstrapUser = $derived(!!profile.current.isBootstrapUser?.());
	// Owners, including the bootstrap user, manage the token, so the identity provider can be set up
	// before any Owner has signed in through it. Only an Owner who signed in through it can enforce.
	// Only Owners can enable SCIM, or delete unreferenced groups.
	let canManageToken = $derived(isOwner);
	let canEnforce = $derived(isOwner && !isBootstrapUser);
	let canEnable = $derived(isOwner);
	let canDeleteGroups = $derived(isOwner);

	let loading = $state(false);
	let refreshing = $state(false);
	let actionError = $state<string>();
	let notice = $state<string>();
	// Why Enable could not delete the unreferenced groups. SCIM is enabled regardless.
	let deletionError = $state<string>();
	// Set once SCIM is enabled, so that the tab never offers to enable it again, even when loading the
	// connection's review failed.
	let enabled = $state(false);
	let confirmEnable = $state(false);
	let confirmEnforce = $state(false);
	let confirmDeleteGroups = $state(false);
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

	let enableNote = $derived(
		`This cannot be undone. Obot stops fetching groups from ${providerName} at sign-in, so until ${providerName} pushes a referenced group, it keeps its current members, including anyone removed from it in ${providerName}. The bearer token for ${providerName} is shown next, only once.`
	);
	let enforceNote = $derived.by(() => {
		const unprovisioned = pages.unprovisionedUsers.total;
		if (unprovisioned === 0) {
			return `This cannot be undone. Only provisioned users can sign in with ${providerName} afterwards.`;
		}
		return `This cannot be undone. Enforcing disables ${count(unprovisioned, 'user')} that ${providerName} has not provisioned, and only provisioned users can sign in with ${providerName} afterwards.`;
	});

	// The steps that finish setting up a connection that is not enforced yet, one at a time. Each must
	// be done before the next one. Enforcing is the last, after which they are no longer shown.
	let setupSteps = $derived.by((): SetupStep[] => {
		if (!review || connection?.state !== 'connected') return [];
		return [
			{
				id: 'token',
				label: 'Token',
				title: 'Generate the bearer token',
				done: connection.hasToken
			},
			{
				id: 'app',
				label: 'SCIM app',
				title: `Create the SCIM app in ${providerName}`,
				done: !!review.activity.lastRequestAt
			},
			{
				id: 'users',
				label: 'Users',
				title: 'Assign users',
				done: pages.provisionedUsers.total > 0
			},
			{
				id: 'groups',
				label: 'Groups',
				// A migrated connection's referenced groups must be pushed, under the names Obot has for them.
				title: connection.origin === 'migrated' ? 'Push the referenced groups' : 'Push groups',
				done: pages.unboundReferencedGroups.total === 0
			},
			{
				id: 'enforce',
				label: 'Enforce',
				title: 'Enforce SCIM',
				done: false
			}
		];
	});
	// The last step that can be shown: the first one that is not done. Only the steps before it show as
	// done, so that a step done ahead of those before it, such as pushing groups to a SCIM-first
	// connection, does not.
	let lastReachableStep = $derived(
		Math.max(
			setupSteps.findIndex((step) => !step.done),
			0
		)
	);
	// The step chosen with Next, Back, or the progress indicator. Until one is chosen, the first step
	// that is not done is shown. A step after one that is no longer done cannot be shown.
	let chosenStep = $state<SetupStepID>();
	let stepIndex = $derived.by(() => {
		const chosen = setupSteps.findIndex((step) => step.id === chosenStep);
		return chosen < 0 ? lastReachableStep : Math.min(chosen, lastReachableStep);
	});
	let currentStep = $derived<SetupStep | undefined>(setupSteps[stepIndex]);
	// Groups pushed under some names bind to no group until the unreferenced groups are deleted, so
	// setup offers to delete them, unless the report of a failed deletion already does.
	let offerGroupDeletion = $derived(
		!deletionError &&
			pages.unreferencedGroups.total > 0 &&
			!!review?.warnings.some(
				(warning) => warning.type === 'duplicateName' || warning.type === 'unreferencedNamesake'
			)
	);
	// Counts a noun, as in "1 group" or "2 groups".
	function count(n: number, singular: string, plural = `${singular}s`) {
		return `${n} ${n === 1 ? singular : plural}`;
	}

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

	function timeAgo(timestamp?: string) {
		return formatTimeAgo(timestamp).relativeTime || 'Never';
	}

	function tokenExpiry(conn: SCIMConnection) {
		return conn.hasToken ? tokenExpiryState(conn.tokenExpiresAt) : undefined;
	}

	function formatDate(timestamp?: string) {
		return timestamp ? new Date(timestamp).toLocaleDateString() : '';
	}

	// Shows a token just issued. The confirmation that issued it closes first, so that the token's
	// dialog is the only one open, and keeps the focus.
	async function showToken(title: string, conn: SCIMConnection) {
		if (!conn.token) return;
		issuedToken = { title, baseURL: conn.baseURL, token: conn.token };
		confirmEnable = false;
		confirmTokenAction = undefined;
		await tick();
		tokenDialog?.open();
	}

	// Loads the review of the connection with the given ID, of the one shown, or of one created
	// since, such as by an Enable that failed after creating it. Without one, it loads the preview of
	// enabling SCIM.
	async function refresh(id = connection?.id) {
		reviewGeneration++;
		id ??= (await AdminService.listSCIMConnections(quiet))[0]?.id;
		if (id) {
			applyReview(await AdminService.getSCIMConnectionReview(id, { limit: pageSize, ...quiet }));
		} else {
			enablePreview = await AdminService.getSCIMEnablePreview(quiet);
		}
	}

	// Keeps the step shown, before what decides whether steps are done changes, so that the step
	// does not move on by itself once it is done, or back to a step it was moved back from.
	function pinStep() {
		chosenStep = currentStep?.id;
	}

	// Shows a review loaded again.
	function applyReview(next: SCIMConnectionReview) {
		pinStep();
		review = next;
		enablePreview = undefined;
		pages = pagesFrom(next);
	}

	async function showPage(list: PagedList, offset: number) {
		// Without a connection, there are no lists.
		if (!connection) return;
		const page = { offset: Math.max(offset, 0), limit: pageSize };
		const request = ++pageRequests[list];
		const generation = reviewGeneration;
		// A response for a page that has since been replaced, by a newer page or by loading the review
		// again, is dropped.
		const current = () => request === pageRequests[list] && generation === reviewGeneration;
		const lastPage = (total: number) => Math.floor((total - 1) / pageSize) * pageSize;
		// The list shrank since it was shown, so the page is past its end: show its last page instead.
		// The last page must come before this one, so that a page that keeps coming back empty is
		// shown empty rather than asked for again forever.
		const pastEnd = (result: SCIMPage<unknown>) =>
			result.items.length === 0 && result.total > 0 && lastPage(result.total) < page.offset;

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
				// A page's total can finish a step, such as pushing the referenced groups.
				pinStep();
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
				// A page's total can finish a step, such as pushing the referenced groups.
				pinStep();
				pages[list] = { ...result, offset: page.offset };
			}
		} catch (err) {
			if (current()) actionError = parseErrorContent(err).message;
		}
	}

	// Loads the review again, for what has changed in the identity provider since.
	async function handleRefresh() {
		loading = true;
		refreshing = true;
		actionError = undefined;
		try {
			await refresh();
		} catch (err) {
			actionError = parseErrorContent(err).message;
		} finally {
			loading = false;
			refreshing = false;
		}
	}

	async function handleEnable() {
		loading = true;
		actionError = undefined;
		notice = undefined;
		deletionError = undefined;
		try {
			const result = await AdminService.enableSCIM();
			enabled = true;
			notice = `SCIM is enabled for ${providerName}.`;
			deletionError = result.deletionError;
			await showToken('SCIM token', result.connection);
			try {
				await refresh(result.connection.id);
			} catch (err) {
				actionError = parseErrorContent(err).message;
			}
			// The layout asks Owners to finish moving to SCIM.
			void adminConfigStore.refresh();
		} catch (err) {
			actionError = parseErrorContent(err).message;
			// Whatever refused Enable may have changed what the preview shows, and an Enable that failed
			// late may have created the connection, whose token can be generated below.
			try {
				await refresh();
				if (connection) void adminConfigStore.refresh();
			} catch {
				// The error above explains what happened.
			}
		} finally {
			confirmEnable = false;
			loading = false;
		}
	}

	async function handleDeleteUnreferencedGroups() {
		if (!connection) return;
		loading = true;
		actionError = undefined;
		notice = undefined;
		try {
			const result = await AdminService.deleteUnreferencedSCIMGroups(connection.id);
			notice = `Deleted ${count(result.deletedGroupCount, 'unreferenced group')}.`;
			deletionError = undefined;
			await refresh();
		} catch (err) {
			actionError = parseErrorContent(err).message;
			try {
				await refresh();
			} catch {
				// The error above explains what happened.
			}
		} finally {
			confirmDeleteGroups = false;
			loading = false;
		}
	}

	async function handleEnforce() {
		if (!connection) return;
		loading = true;
		actionError = undefined;
		notice = undefined;
		try {
			const result = await AdminService.enforceSCIM(connection.id);
			// Enforcing deleted the unreferenced groups that enabling could not.
			deletionError = undefined;
			notice =
				result.disabledUserCount > 0
					? `SCIM is enforced. Disabled ${count(result.disabledUserCount, 'unprovisioned user')}.`
					: 'SCIM is enforced.';
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
					await showToken('SCIM token', await AdminService.rotateSCIMToken(connection.id, quiet));
					break;
				case 'rotate':
					await showToken(
						'New SCIM token',
						await AdminService.rotateSCIMToken(connection.id, quiet)
					);
					break;
				case 'revokeCurrent':
					await showToken(
						'Replacement SCIM token',
						await AdminService.revokeCurrentSCIMToken(connection.id, quiet)
					);
					break;
				case 'revokePrevious':
					await AdminService.revokePreviousSCIMToken(connection.id, quiet);
					break;
			}
			// The layout warns Owners of a token that expires soon, which a new token replaces.
			if (action !== 'revokePrevious') void adminConfigStore.refresh();
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
			note: 'The token is shown only once. Copy it into the SCIM application of the identity provider. It expires after a year, so rotate it before then.',
			submit: 'Generate token',
			type: 'info'
		},
		rotate: {
			title: 'Rotate token',
			msg: 'Issue a new SCIM bearer token?',
			note: 'The current token keeps working for a day, or until it expires or you revoke it, so you can update the identity provider without failed requests.',
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

	{#if deletionError && (!connection || pages.unreferencedGroups.total > 0)}
		<div class="notification-alert flex flex-wrap items-start gap-2" role="alert">
			<TriangleAlert class="mt-0.5 size-5 shrink-0" />
			<p class="min-w-0 flex-1 text-sm font-light">{deletionError}</p>
			{#if connection && canDeleteGroups}
				{@render deleteGroupsButton(true)}
			{/if}
		</div>
	{/if}

	{#if connection && review}
		{@render connectionDetails(connection)}
		{#if connection.state === 'connected'}
			{@render setupWizard(review, connection)}
		{:else}
			{@render groupsSection(review)}
			{@render usersSection()}
		{/if}
		{@render activitySection(review)}
	{:else if enabled}
		<section class="paper" aria-labelledby="scim-enabled-title">
			<h2 id="scim-enabled-title" class="text-lg font-semibold">SCIM provisioning is enabled</h2>
			<p class="text-muted-content text-sm font-light">
				Reload this page to review the move to SCIM.
			</p>
		</section>
	{:else if offerEnable && enablePreview}
		{@render enableSection(enablePreview)}
	{:else}
		<section class="paper" aria-labelledby="scim-none-title">
			<h2 id="scim-none-title" class="text-lg font-semibold">SCIM provisioning is not set up</h2>
			<p class="text-muted-content text-sm font-light">
				To provision users and groups through SCIM, configure an auth provider that supports it,
				such as Okta, on the Providers tab, and leave its directory credentials (the API Services
				client ID and private key) empty. Setup then continues here once an Owner has signed in.
			</p>
			<p class="text-muted-content text-sm font-light">
				An auth provider that supports SCIM and already fetches groups from its directory at sign-in
				can be moved to SCIM here.
			</p>
			{#if enablePreview?.blockers.length}
				{@render blockerList('SCIM cannot be enabled', enablePreview.blockers)}
			{/if}
		</section>
	{/if}
</div>

{#snippet connectionDetails(conn: SCIMConnection)}
	<section class="paper" aria-labelledby="scim-connection-title">
		<div class="flex flex-wrap items-center gap-2">
			<h2 id="scim-connection-title" class="text-lg font-semibold">SCIM provisioning</h2>
			<span class={conn.state === 'enforced' ? 'pill-primary' : 'pill-warning'}>
				{conn.state === 'enforced' ? 'Enforced' : 'Not finished'}
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
					{#if conn.hasToken}
						{providerName} is not the configured auth provider, so SCIM requests fail, and its token cannot
						be rotated or replaced.
					{:else}
						{providerName} is not the configured auth provider yet, so SCIM requests fail. The token can
						be generated once it serves sign-ins.
					{/if}
				</span>
			</div>
		{/if}

		<!-- Its token cannot be rotated while the provider is not configured, which the alert above says. -->
		{#if conn.hasToken && conn.authProviderConfigured}
			{@const expiry = tokenExpiry(conn)}
			{#if expiry === 'expired'}
				<div class="notification-error flex items-start gap-2 text-sm font-light" role="alert">
					<CircleAlert class="text-error mt-0.5 size-5 shrink-0" />
					<span>
						The bearer token expired on {formatDate(conn.tokenExpiresAt)}, so {providerName}'s SCIM
						requests fail. Rotate the token, update it in {providerName}, and retry the failed
						provisioning tasks there.
					</span>
				</div>
			{:else if expiry === 'expiring'}
				<div class="notification-alert flex items-start gap-2 text-sm font-light" role="status">
					<TriangleAlert class="mt-0.5 size-5 shrink-0" />
					<span>
						The bearer token expires on {formatDate(conn.tokenExpiresAt)}. Rotate it before then,
						and update it in {providerName}: the current token keeps working for up to a day after
						the rotation.
					</span>
				</div>
			{/if}
		{/if}

		<dl class="grid grid-cols-1 gap-4 text-sm md:grid-cols-2">
			<div class="flex min-w-0 flex-col gap-1 md:col-span-2">
				<dt class="text-muted-content text-xs">Base URL</dt>
				<dd class="min-w-0">{@render copyableValue(conn.baseURL, 'Copy base URL')}</dd>
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
						{#if conn.tokenExpiresAt}
							<span class="text-muted-content block text-xs">
								{`${tokenExpiry(conn) === 'expired' ? 'Expired' : 'Expires'} ${formatDate(conn.tokenExpiresAt)}`}
							</span>
						{/if}
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

		{#if canManageToken && conn.hasToken}
			<div class="flex flex-wrap justify-end gap-2">
				{#if conn.previousTokenAccepted}
					<button
						class="btn btn-secondary"
						disabled={loading}
						onclick={() => (confirmTokenAction = 'revokePrevious')}
					>
						Revoke previous token
					</button>
				{/if}
				<!-- The server issues tokens only for the configured auth provider. -->
				{#if conn.authProviderConfigured}
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
				{/if}
			</div>
		{/if}
	</section>
{/snippet}

{#snippet setupWizard(r: SCIMConnectionReview, conn: SCIMConnection)}
	<section class="paper" aria-labelledby="scim-setup-title">
		<div class="flex items-center justify-between gap-2">
			<h2 id="scim-setup-title" class="text-lg font-semibold">
				{conn.origin === 'migrated' ? 'Finish moving to SCIM' : 'Set up provisioning'}
			</h2>
			<button
				class="text-muted-content hover:bg-base-300 hover:text-base-content rounded-md p-1 disabled:opacity-50"
				use:tooltip={'Refresh'}
				aria-label="Refresh"
				disabled={loading}
				onclick={handleRefresh}
			>
				<RefreshCw class={['size-4', refreshing && 'animate-spin']} />
			</button>
		</div>

		<ol class="flex flex-wrap items-center gap-x-3 gap-y-2" aria-label="Setup steps">
			{#each setupSteps as step, index (step.id)}
				{@const done = index < lastReachableStep}
				<li class="flex items-center gap-3" aria-current={index === stepIndex ? 'step' : undefined}>
					<button
						class="flex items-center gap-1.5 text-xs disabled:cursor-default"
						aria-label={done ? `${step.label}, done` : step.label}
						disabled={index > lastReachableStep || index === stepIndex}
						onclick={() => (chosenStep = step.id)}
					>
						{#if done}
							<CircleCheck class="text-success size-4 shrink-0" aria-hidden="true" />
						{:else}
							<span
								class={[
									'flex size-4 shrink-0 items-center justify-center rounded-full text-[10px]',
									index === stepIndex
										? 'bg-primary text-white'
										: 'border-base-content/30 text-muted-content border'
								]}
								aria-hidden="true">{index + 1}</span
							>
						{/if}
						<span class={index === stepIndex ? 'font-medium' : 'text-muted-content'}>
							{step.label}
						</span>
					</button>
					{#if index < setupSteps.length - 1}
						<span class="bg-base-300 dark:bg-base-400 h-px w-6" aria-hidden="true"></span>
					{/if}
				</li>
			{/each}
		</ol>

		{#if currentStep}
			<div class="flex flex-col gap-3" role="group" aria-labelledby="scim-step-title">
				<h3 id="scim-step-title" class="text-base font-semibold">{currentStep.title}</h3>
				{#if currentStep.id === 'token'}
					{@render tokenStep(conn)}
				{:else if currentStep.id === 'app'}
					{@render appStep(r)}
				{:else if currentStep.id === 'users'}
					{@render usersStep()}
				{:else if currentStep.id === 'groups'}
					{@render groupsStep(r, conn)}
				{:else}
					{@render enforceStep(r)}
				{/if}
			</div>

			<div class="flex flex-wrap items-center justify-end gap-2">
				{#if stepIndex > 0}
					<button
						class="btn btn-secondary"
						onclick={() => (chosenStep = setupSteps[stepIndex - 1].id)}
					>
						Back
					</button>
				{/if}
				{#if currentStep.id === 'enforce'}
					<button
						class="btn btn-primary"
						disabled={!canEnforce || loading || r.enforceBlockers.length > 0}
						onclick={() => (confirmEnforce = true)}
					>
						Enforce SCIM
					</button>
				{:else}
					<button
						class="btn btn-primary"
						disabled={!currentStep.done}
						onclick={() => (chosenStep = setupSteps[stepIndex + 1].id)}
					>
						Next
					</button>
				{/if}
			</div>
		{/if}
	</section>
{/snippet}

{#snippet stepStatus(done: boolean, text: string)}
	<p class="flex items-start gap-2 text-sm">
		{#if done}
			<CircleCheck class="text-success mt-0.5 size-4 shrink-0" aria-hidden="true" />
		{:else}
			<Circle class="text-muted-content mt-0.5 size-4 shrink-0" aria-hidden="true" />
		{/if}
		{text}
	</p>
{/snippet}

{#snippet tokenStep(conn: SCIMConnection)}
	<p class="text-muted-content text-sm font-light">
		{providerName} sends the bearer token with each SCIM request. Obot shows it only once, when it is
		issued.
	</p>
	{#if conn.hasToken && tokenExpiry(conn) === 'expired'}
		{@render stepStatus(
			false,
			conn.authProviderConfigured
				? `The token expired on ${formatDate(conn.tokenExpiresAt)}. Rotate it, and update it in ${providerName}.`
				: `The token expired on ${formatDate(conn.tokenExpiresAt)}.`
		)}
	{:else if conn.hasToken}
		{@render stepStatus(true, `The token was issued ${timeAgo(conn.tokenIssuedAt)}.`)}
	{:else if canManageToken && conn.authProviderConfigured}
		<div>
			<button
				class="btn btn-primary"
				disabled={loading}
				onclick={() => (confirmTokenAction = 'generate')}
			>
				Generate token
			</button>
		</div>
	{:else if canManageToken}
		{@render stepStatus(
			false,
			`The token can be generated once ${providerName} is the configured auth provider.`
		)}
	{:else}
		{@render stepStatus(false, 'An Owner generates the token.')}
	{/if}
{/snippet}

{#snippet appStep(r: SCIMConnectionReview)}
	<p class="text-muted-content text-sm font-light">
		In {providerName}, create a SCIM 2.0 application, and enter the base URL shown above and the
		bearer token. This step is done once {providerName} sends Obot a request.
	</p>
	{#if r.activity.lastRequestAt}
		{@render stepStatus(
			true,
			`${providerName} sent its last request ${timeAgo(r.activity.lastRequestAt)}.`
		)}
	{:else}
		{@render stepStatus(false, `Waiting for ${providerName} to send a request.`)}
	{/if}
	{#if pages.failures.total > 0}
		<p class="text-muted-content text-xs font-light">
			{count(pages.failures.total, 'recent request')} failed, as listed under Activity.
		</p>
	{/if}
{/snippet}

{#snippet usersStep()}
	<p class="text-muted-content text-sm font-light">
		Assign everyone who should be able to sign in to the SCIM application in {providerName}, which
		then provisions them in Obot.
	</p>
	{@render stepStatus(
		pages.provisionedUsers.total > 0,
		`${pages.provisionedUsers.total} provisioned, ${pages.unprovisionedUsers.total} not provisioned yet.`
	)}
	{#if pages.unprovisionedUsers.total > 0}
		<div class="flex flex-col gap-2">
			<h4 class="text-sm font-semibold">Not provisioned ({pages.unprovisionedUsers.total})</h4>
			<p class="text-muted-content text-xs font-light">
				Enforcing SCIM disables these users. Provisioning them later re-enables them with their
				account and data.
			</p>
			{@render userList('unprovisionedUsers', '')}
		</div>
	{/if}
{/snippet}

{#snippet groupsStep(r: SCIMConnectionReview, conn: SCIMConnection)}
	<p class="text-muted-content text-sm font-light">
		{#if conn.origin === 'migrated'}
			Push each referenced group from {providerName} under exactly the name shown, renaming it there first
			if needed, or remove its references.
		{:else}
			Push the groups from {providerName} that roles and policies should be granted to. More can be pushed
			at any time.
		{/if}
	</p>
	{@render warnings(r.warnings)}
	{#if offerGroupDeletion && canDeleteGroups}
		<div>{@render deleteGroupsButton(false)}</div>
	{/if}
	{#if pages.unboundReferencedGroups.total > 0}
		{@render stepStatus(
			false,
			`${count(pages.unboundReferencedGroups.total, 'referenced group')} not pushed yet.`
		)}
		{@render groupList('unboundReferencedGroups', '')}
	{:else if conn.origin === 'migrated'}
		{@render stepStatus(true, 'Every referenced group has been pushed.')}
	{:else}
		{@render stepStatus(
			true,
			pages.boundGroups.total > 0
				? `${count(pages.boundGroups.total, 'group')} pushed.`
				: 'No groups have been pushed yet.'
		)}
	{/if}
	{#if pages.boundGroups.total > 0}
		<div class="flex flex-col gap-2">
			<h4 class="text-sm font-semibold">Pushed groups ({pages.boundGroups.total})</h4>
			{@render groupList('boundGroups', '')}
		</div>
	{/if}
{/snippet}

{#snippet enforceStep(r: SCIMConnectionReview)}
	<p class="text-muted-content text-sm font-light">
		Once SCIM is enforced, only accounts that {providerName} provisioned can sign in.
		{#if pages.unprovisionedUsers.total > 0}
			Enforcing disables the {count(pages.unprovisionedUsers.total, 'user')} that it has not provisioned.
			Group memberships do not change, and nothing is deleted from the users.
		{/if}
		It cannot be undone.
	</p>
	{#if r.enforceBlockers.length > 0}
		{@render blockerList('SCIM cannot be enforced yet', r.enforceBlockers)}
	{:else}
		{@render stepStatus(true, 'Ready to enforce SCIM.')}
	{/if}
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
					one shown here, or remove its references.
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
				</p>
				{@render groupList('unreferencedGroups', '')}
				{#if canDeleteGroups}
					<div class="flex justify-end">{@render deleteGroupsButton(false)}</div>
				{/if}
			</div>
		{/if}
	</section>
{/snippet}

{#snippet usersSection()}
	<section class="paper" aria-labelledby="scim-users-title">
		<h2 id="scim-users-title" class="text-lg font-semibold">Users</h2>

		<div class="flex flex-col gap-2">
			<h3 class="text-sm font-semibold">Provisioned ({pages.provisionedUsers.total})</h3>
			{@render userList('provisionedUsers', `${providerName} has not provisioned any users yet.`)}
		</div>

		{#if pages.unprovisionedUsers.total > 0}
			<div class="flex flex-col gap-2">
				<h3 class="text-sm font-semibold">Not provisioned ({pages.unprovisionedUsers.total})</h3>
				<p class="text-muted-content text-xs font-light">
					These users cannot sign in until {providerName} provisions them, which re-enables them with
					their account and data.
				</p>
				{@render userList('unprovisionedUsers', '')}
			</div>
		{/if}
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

{#snippet enableSection(p: SCIMEnablePreview)}
	<section class="paper" aria-labelledby="scim-enable-title">
		<div class="flex flex-col gap-1">
			<h2 id="scim-enable-title" class="text-lg font-semibold">Move {providerName} to SCIM</h2>
			<p class="text-muted-content text-sm font-light">
				Obot fetches each user's groups from {providerName} when they sign in. Enabling SCIM lets {providerName}
				provision users, account status, groups, and memberships instead, and Obot stops fetching groups
				at sign-in. It cannot be undone.
			</p>
			<p class="text-muted-content text-sm font-light">
				Everyone can still sign in once SCIM is enabled. Enforcing SCIM, a later step, will allow
				only accounts provisioned by {providerName} to sign in.
			</p>
		</div>

		{#if p.blockers.length > 0}
			{@render blockerList('SCIM cannot be enabled yet', p.blockers)}
		{/if}

		{#each p.duplicateGroupNames as duplicate (duplicate.name)}
			<div class="flex flex-col gap-2">
				<h3 class="text-sm font-semibold">
					Referenced groups named "{duplicate.name}" ({duplicate.groups.length})
				</h3>
				{@render groupItems(duplicate.groups)}
			</div>
		{/each}

		<div class="flex items-center justify-end gap-2">
			{#if !canEnable}
				<p class="text-muted-content text-xs font-light">Only an owner can enable SCIM.</p>
			{/if}
			<button
				class="btn btn-primary"
				disabled={!canEnable || loading || p.blockers.length > 0}
				onclick={() => (confirmEnable = true)}
			>
				Enable SCIM
			</button>
		</div>
	</section>
{/snippet}

{#snippet deleteGroupsButton(small: boolean)}
	<button
		class={['btn btn-secondary', small && 'btn-sm']}
		disabled={loading}
		onclick={() => (confirmDeleteGroups = true)}
	>
		Delete unreferenced groups
	</button>
{/snippet}

{#snippet copyableValue(value: string, tooltipText: string)}
	<div class="flex min-w-0 items-center gap-2">
		<span class="font-mono text-sm break-all">{value}</span>
		<CopyButton text={value} {tooltipText} />
	</div>
{/snippet}

{#snippet blockerList(title: string, items: string[])}
	<div class="notification-alert flex items-start gap-2" role="alert">
		<TriangleAlert class="mt-0.5 size-5 shrink-0" />
		<div class="flex min-w-0 flex-col gap-1">
			<p class="text-sm font-medium">{title}</p>
			<ul class="flex list-disc flex-col gap-1 pl-4 text-sm font-light wrap-break-word">
				{#each items as item (item)}
					<li>{item}</li>
				{/each}
			</ul>
		</div>
	</div>
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
								Referenced by {warning.references.map(describeGroupReference).join('; ')}.
							</span>
						{/if}
					</li>
				{/each}
			</ul>
		</div>
	{/if}
{/snippet}

{#snippet groupList(list: GroupPagedList, empty: string)}
	{#if pages[list].total === 0}
		{#if empty}
			<p class="text-muted-content text-sm font-light">{empty}</p>
		{/if}
	{:else}
		{@render groupItems(pages[list].items)}
		{@render pager(list)}
	{/if}
{/snippet}

{#snippet groupItems(groups: SCIMSetupGroup[])}
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
						Referenced by {group.references.map(describeGroupReference).join('; ')}
					</p>
				{/if}
			</li>
		{/each}
	</ul>
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
	{#if current.total > pageSize || current.offset > 0}
		<Pagination
			pageIndex={Math.floor(current.offset / pageSize)}
			lastPageIndex={Math.max(Math.ceil(current.total / pageSize) - 1, 0)}
			total={current.total}
			{loading}
			label={listNouns[list]}
			onPageChange={(index) => showPage(list, index * pageSize)}
		/>
	{/if}
{/snippet}

<Confirm
	show={confirmEnable}
	title="Enable SCIM"
	msg="Enable SCIM for {providerName}?"
	note={enableNote}
	submitText="Enable SCIM"
	{loading}
	onsuccess={handleEnable}
	oncancel={() => (confirmEnable = false)}
/>

<Confirm
	show={confirmDeleteGroups}
	title="Delete unreferenced groups"
	msg="Delete {count(pages.unreferencedGroups.total, 'unreferenced group')}?"
	note="They grant nothing, so no one loses access. The groups will still exist in {providerName} but will no longer be known to Obot."
	submitText="Delete groups"
	{loading}
	onsuccess={handleDeleteUnreferencedGroups}
	oncancel={() => (confirmDeleteGroups = false)}
/>

<Confirm
	show={confirmEnforce}
	title="Enforce SCIM"
	msg="Enforce SCIM for {providerName}?"
	note={enforceNote}
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
				<TriangleAlert class="mt-0.5 size-5 shrink-0" />
				<span>
					Copy the token now. It is shown only once. Enter the base URL and the token in the SCIM
					application in {providerName}.
				</span>
			</div>
			<div class="flex min-w-0 flex-col gap-1">
				<span class="text-muted-content text-xs">Base URL</span>
				{@render copyableValue(issuedToken.baseURL, 'Copy base URL')}
			</div>
			<div class="flex min-w-0 flex-col gap-1">
				<span class="text-muted-content text-xs">Bearer token</span>
				{@render copyableValue(issuedToken.token, 'Copy token')}
			</div>
			<div class="flex justify-end">
				<button class="btn btn-primary" onclick={() => tokenDialog?.close()}>Done</button>
			</div>
		</div>
	{/if}
</ResponsiveDialog>

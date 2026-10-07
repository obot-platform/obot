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
	import { m } from '$lib/i18n';
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
	let listNouns = $derived<Record<PagedList, string>>({
		provisionedUsers: m.identity_access_scim_list_provisioned_users(),
		unprovisionedUsers: m.identity_access_scim_list_unprovisioned_users(),
		boundGroups: m.identity_access_scim_list_pushed_groups(),
		unboundReferencedGroups: m.identity_access_scim_list_unpushed_groups(),
		unreferencedGroups: m.identity_access_scim_list_unreferenced_groups(),
		failures: m.identity_access_scim_list_failures()
	});

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
			m.identity_access_scim_provider_fallback()
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

	let enableNote = $derived(m.identity_access_scim_enable_note({ provider: providerName }));
	let enforceNote = $derived.by(() => {
		const unprovisioned = pages.unprovisionedUsers.total;
		if (unprovisioned === 0) {
			return m.identity_access_scim_enforce_note_none({ provider: providerName });
		}
		return m.identity_access_scim_enforce_note_some({
			provider: providerName,
			users: counted(
				unprovisioned,
				m.identity_access_scim_count_user_one,
				m.identity_access_scim_count_user_other
			)
		});
	});

	// The steps that finish setting up a connection that is not enforced yet, one at a time. Each must
	// be done before the next one. Enforcing is the last, after which they are no longer shown.
	let setupSteps = $derived.by((): SetupStep[] => {
		if (!review || connection?.state !== 'connected') return [];
		return [
			{
				id: 'token',
				label: m.identity_access_scim_step_token(),
				title: m.identity_access_scim_step_token_title(),
				done: connection.hasToken
			},
			{
				id: 'app',
				label: m.identity_access_scim_step_app(),
				title: m.identity_access_scim_step_app_title({ provider: providerName }),
				done: !!review.activity.lastRequestAt
			},
			{
				id: 'users',
				label: m.identity_access_users_tab(),
				title: m.identity_access_scim_step_assign_users(),
				done: pages.provisionedUsers.total > 0
			},
			{
				id: 'groups',
				label: m.identity_access_groups_tab(),
				// A migrated connection's referenced groups must be pushed, under the names Obot has for them.
				title:
					connection.origin === 'migrated'
						? m.identity_access_scim_step_push_referenced()
						: m.identity_access_scim_step_push_groups(),
				done: pages.unboundReferencedGroups.total === 0
			},
			{
				id: 'enforce',
				label: m.identity_access_scim_step_enforce(),
				title: m.identity_access_scim_enforce_title(),
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
	function counted(
		n: number,
		one: (args: { count: number }) => string,
		other: (args: { count: number }) => string
	) {
		return (n === 1 ? one : other)({ count: n });
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
		return formatTimeAgo(timestamp).relativeTime || m.identity_access_scim_never();
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
			notice = m.identity_access_scim_enabled_notice({ provider: providerName });
			deletionError = result.deletionError;
			await showToken(m.identity_access_scim_token_title(), result.connection);
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
			notice = m.identity_access_scim_deleted_groups({
				groups: counted(
					result.deletedGroupCount,
					m.identity_access_scim_count_unreferenced_group_one,
					m.identity_access_scim_count_unreferenced_group_other
				)
			});
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
					? m.identity_access_scim_enforced_disabled({
							users: counted(
								result.disabledUserCount,
								m.identity_access_scim_count_unprovisioned_user_one,
								m.identity_access_scim_count_unprovisioned_user_other
							)
						})
					: m.identity_access_scim_enforced_notice();
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
					await showToken(
						m.identity_access_scim_token_title(),
						await AdminService.rotateSCIMToken(connection.id, quiet)
					);
					break;
				case 'rotate':
					await showToken(
						m.identity_access_scim_new_token(),
						await AdminService.rotateSCIMToken(connection.id, quiet)
					);
					break;
				case 'revokeCurrent':
					await showToken(
						m.identity_access_scim_replacement_token(),
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

	let tokenConfirmations = $derived<
		Record<
			TokenAction,
			{ title: string; msg: string; note: string; submit: string; type: 'info' | 'delete' }
		>
	>({
		generate: {
			title: m.identity_access_scim_generate_token(),
			msg: m.identity_access_scim_issue_token_msg(),
			note: m.identity_access_scim_issue_token_note(),
			submit: m.identity_access_scim_generate_token(),
			type: 'info'
		},
		rotate: {
			title: m.identity_access_scim_rotate_token(),
			msg: m.identity_access_scim_rotate_token_msg(),
			note: m.identity_access_scim_rotate_token_note(),
			submit: m.identity_access_scim_rotate_token(),
			type: 'info'
		},
		revokePrevious: {
			title: m.identity_access_scim_revoke_previous(),
			msg: m.identity_access_scim_revoke_previous_msg(),
			note: m.identity_access_scim_revoke_previous_note(),
			submit: m.identity_access_scim_revoke(),
			type: 'delete'
		},
		revokeCurrent: {
			title: m.identity_access_scim_revoke_current(),
			msg: m.identity_access_scim_revoke_current_msg(),
			note: m.identity_access_scim_revoke_current_note(),
			submit: m.identity_access_scim_revoke_replace(),
			type: 'delete'
		}
	});
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
			<h2 id="scim-enabled-title" class="text-lg font-semibold">
				{m.identity_access_scim_enabled_title()}
			</h2>
			<p class="text-muted-content text-sm font-light">{m.identity_access_scim_reload()}</p>
		</section>
	{:else if offerEnable && enablePreview}
		{@render enableSection(enablePreview)}
	{:else}
		<section class="paper" aria-labelledby="scim-none-title">
			<h2 id="scim-none-title" class="text-lg font-semibold">
				{m.identity_access_scim_not_setup_title()}
			</h2>
			<p class="text-muted-content text-sm font-light">{m.identity_access_scim_not_setup_body()}</p>
			<p class="text-muted-content text-sm font-light">{m.identity_access_scim_not_setup_move()}</p>
			{#if enablePreview?.blockers.length}
				{@render blockerList(m.identity_access_scim_cannot_enable(), enablePreview.blockers)}
			{/if}
		</section>
	{/if}
</div>

{#snippet connectionDetails(conn: SCIMConnection)}
	<section class="paper" aria-labelledby="scim-connection-title">
		<div class="flex flex-wrap items-center gap-2">
			<h2 id="scim-connection-title" class="text-lg font-semibold">
				{m.identity_access_scim_provisioning()}
			</h2>
			<span class={conn.state === 'enforced' ? 'pill-primary' : 'pill-warning'}>
				{conn.state === 'enforced'
					? m.identity_access_scim_enforced()
					: m.identity_access_scim_not_finished()}
			</span>
		</div>
		<p class="text-muted-content text-sm font-light">
			{#if conn.state === 'enforced'}
				{m.identity_access_scim_enforced_body({ provider: providerName })}
			{:else}
				{m.identity_access_scim_connected_body({ provider: providerName })}
			{/if}
		</p>

		{#if !conn.authProviderConfigured}
			<div class="notification-alert flex items-start gap-2 text-sm font-light" role="alert">
				<TriangleAlert class="mt-0.5 size-5 shrink-0" />
				<span>
					{#if conn.hasToken}
						{m.identity_access_scim_provider_not_configured_token({ provider: providerName })}
					{:else}
						{m.identity_access_scim_provider_not_configured({ provider: providerName })}
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
						{m.identity_access_scim_bearer_expired({
							date: formatDate(conn.tokenExpiresAt),
							provider: providerName
						})}
					</span>
				</div>
			{:else if expiry === 'expiring'}
				<div class="notification-alert flex items-start gap-2 text-sm font-light" role="status">
					<TriangleAlert class="mt-0.5 size-5 shrink-0" />
					<span>
						{m.identity_access_scim_bearer_expiring({
							date: formatDate(conn.tokenExpiresAt),
							provider: providerName
						})}
					</span>
				</div>
			{/if}
		{/if}

		<dl class="grid grid-cols-1 gap-4 text-sm md:grid-cols-2">
			<div class="flex min-w-0 flex-col gap-1 md:col-span-2">
				<dt class="text-muted-content text-xs">{m.identity_access_scim_base_url()}</dt>
				<dd class="min-w-0">
					{@render copyableValue(conn.baseURL, m.identity_access_scim_copy_base_url())}
				</dd>
			</div>
			<div class="flex flex-col gap-1">
				<dt class="text-muted-content text-xs">{m.identity_access_scim_auth_provider()}</dt>
				<dd>{providerName}</dd>
			</div>
			<div class="flex flex-col gap-1">
				<dt class="text-muted-content text-xs">{m.identity_access_scim_bearer_token()}</dt>
				<dd>
					{#if conn.hasToken}
						{m.identity_access_scim_issued({ time: timeAgo(conn.tokenIssuedAt) })}
						{#if conn.tokenExpiresAt}
							<span class="text-muted-content block text-xs">
								{tokenExpiry(conn) === 'expired'
									? m.identity_access_scim_expired_on({ date: formatDate(conn.tokenExpiresAt) })
									: m.identity_access_scim_expires_on({ date: formatDate(conn.tokenExpiresAt) })}
							</span>
						{/if}
					{:else}
						{m.identity_access_scim_not_generated()}
					{/if}
				</dd>
			</div>
			{#if conn.previousTokenAccepted}
				<div class="flex flex-col gap-1">
					<dt class="text-muted-content text-xs">{m.identity_access_scim_previous_token()}</dt>
					<dd>
						{m.identity_access_scim_accepted_until({
							time: new Date(conn.previousTokenExpiresAt ?? '').toLocaleString()
						})}
					</dd>
				</div>
			{/if}
			<div class="flex flex-col gap-1">
				<dt class="text-muted-content text-xs">{m.identity_access_scim_enforced_label()}</dt>
				<dd>{conn.enforcedAt ? timeAgo(conn.enforcedAt) : m.identity_access_scim_not_yet()}</dd>
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
						{m.identity_access_scim_revoke_previous()}
					</button>
				{/if}
				<!-- The server issues tokens only for the configured auth provider. -->
				{#if conn.authProviderConfigured}
					<button
						class="btn btn-secondary"
						disabled={loading}
						onclick={() => (confirmTokenAction = 'revokeCurrent')}
					>
						{m.identity_access_scim_revoke_current()}
					</button>
					<button
						class="btn btn-secondary"
						disabled={loading}
						onclick={() => (confirmTokenAction = 'rotate')}
					>
						{m.identity_access_scim_rotate_token()}
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
				{conn.origin === 'migrated'
					? m.identity_access_scim_finish_move()
					: m.identity_access_scim_setup_provisioning()}
			</h2>
			<button
				class="text-muted-content hover:bg-base-300 hover:text-base-content rounded-md p-1 disabled:opacity-50"
				use:tooltip={m.platform_refresh()}
				aria-label={m.platform_refresh()}
				disabled={loading}
				onclick={handleRefresh}
			>
				<RefreshCw class={['size-4', refreshing && 'animate-spin']} />
			</button>
		</div>

		<ol
			class="flex flex-wrap items-center gap-x-3 gap-y-2"
			aria-label={m.identity_access_scim_setup_steps()}
		>
			{#each setupSteps as step, index (step.id)}
				{@const done = index < lastReachableStep}
				<li class="flex items-center gap-3" aria-current={index === stepIndex ? 'step' : undefined}>
					<button
						class="flex items-center gap-1.5 text-xs disabled:cursor-default"
						aria-label={done ? m.identity_access_scim_step_done({ label: step.label }) : step.label}
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
						{m.common_back()}
					</button>
				{/if}
				{#if currentStep.id === 'enforce'}
					<button
						class="btn btn-primary"
						disabled={!canEnforce || loading || r.enforceBlockers.length > 0}
						onclick={() => (confirmEnforce = true)}
					>
						{m.identity_access_scim_enforce_title()}
					</button>
				{:else}
					<button
						class="btn btn-primary"
						disabled={!currentStep.done}
						onclick={() => (chosenStep = setupSteps[stepIndex + 1].id)}
					>
						{m.core_next()}
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
		{m.identity_access_scim_token_help({ provider: providerName })}
	</p>
	{#if conn.hasToken && tokenExpiry(conn) === 'expired'}
		{@render stepStatus(
			false,
			conn.authProviderConfigured
				? m.identity_access_scim_token_expired_rotate({
						date: formatDate(conn.tokenExpiresAt),
						provider: providerName
					})
				: m.identity_access_scim_token_expired_on({ date: formatDate(conn.tokenExpiresAt) })
		)}
	{:else if conn.hasToken}
		{@render stepStatus(
			true,
			m.identity_access_scim_token_issued({ time: timeAgo(conn.tokenIssuedAt) })
		)}
	{:else if canManageToken && conn.authProviderConfigured}
		<div>
			<button
				class="btn btn-primary"
				disabled={loading}
				onclick={() => (confirmTokenAction = 'generate')}
			>
				{m.identity_access_scim_generate_token()}
			</button>
		</div>
	{:else if canManageToken}
		{@render stepStatus(
			false,
			m.identity_access_scim_token_when_configured({ provider: providerName })
		)}
	{:else}
		{@render stepStatus(false, m.identity_access_scim_owner_generates())}
	{/if}
{/snippet}

{#snippet appStep(r: SCIMConnectionReview)}
	<p class="text-muted-content text-sm font-light">
		{m.identity_access_scim_app_help({ provider: providerName })}
	</p>
	{#if r.activity.lastRequestAt}
		{@render stepStatus(
			true,
			m.identity_access_scim_last_request_sent({
				provider: providerName,
				time: timeAgo(r.activity.lastRequestAt)
			})
		)}
	{:else}
		{@render stepStatus(false, m.identity_access_scim_waiting_request({ provider: providerName }))}
	{/if}
	{#if pages.failures.total > 0}
		<p class="text-muted-content text-xs font-light">
			{m.identity_access_scim_requests_failed({
				requests: counted(
					pages.failures.total,
					m.identity_access_scim_count_recent_request_one,
					m.identity_access_scim_count_recent_request_other
				)
			})}
		</p>
	{/if}
{/snippet}

{#snippet usersStep()}
	<p class="text-muted-content text-sm font-light">
		{m.identity_access_scim_assign_help({ provider: providerName })}
	</p>
	{@render stepStatus(
		pages.provisionedUsers.total > 0,
		m.identity_access_scim_provisioned_counts({
			provisioned: pages.provisionedUsers.total,
			unprovisioned: pages.unprovisionedUsers.total
		})
	)}
	{#if pages.unprovisionedUsers.total > 0}
		<div class="flex flex-col gap-2">
			<h4 class="text-sm font-semibold">
				{m.identity_access_scim_not_provisioned_heading({ count: pages.unprovisionedUsers.total })}
			</h4>
			<p class="text-muted-content text-xs font-light">
				{m.identity_access_scim_enforce_disables()}
			</p>
			{@render userList('unprovisionedUsers', '')}
		</div>
	{/if}
{/snippet}

{#snippet groupsStep(r: SCIMConnectionReview, conn: SCIMConnection)}
	<p class="text-muted-content text-sm font-light">
		{#if conn.origin === 'migrated'}
			{m.identity_access_scim_groups_migrated({ provider: providerName })}
		{:else}
			{m.identity_access_scim_groups_new({ provider: providerName })}
		{/if}
	</p>
	{@render warnings(r.warnings)}
	{#if offerGroupDeletion && canDeleteGroups}
		<div>{@render deleteGroupsButton(false)}</div>
	{/if}
	{#if pages.unboundReferencedGroups.total > 0}
		{@render stepStatus(
			false,
			m.identity_access_scim_not_pushed_yet({
				groups: counted(
					pages.unboundReferencedGroups.total,
					m.identity_access_scim_count_referenced_group_one,
					m.identity_access_scim_count_referenced_group_other
				)
			})
		)}
		{@render groupList('unboundReferencedGroups', '')}
	{:else if conn.origin === 'migrated'}
		{@render stepStatus(true, m.identity_access_scim_every_group_pushed())}
	{:else}
		{@render stepStatus(
			true,
			pages.boundGroups.total > 0
				? m.identity_access_scim_groups_pushed({
						groups: counted(
							pages.boundGroups.total,
							m.identity_access_scim_count_group_one,
							m.identity_access_scim_count_group_other
						)
					})
				: m.identity_access_scim_no_groups_pushed()
		)}
	{/if}
	{#if pages.boundGroups.total > 0}
		<div class="flex flex-col gap-2">
			<h4 class="text-sm font-semibold">
				{m.identity_access_scim_pushed_groups_heading({ count: pages.boundGroups.total })}
			</h4>
			{@render groupList('boundGroups', '')}
		</div>
	{/if}
{/snippet}

{#snippet enforceStep(r: SCIMConnectionReview)}
	<p class="text-muted-content text-sm font-light">
		{m.identity_access_scim_enforce_help({ provider: providerName })}
		{#if pages.unprovisionedUsers.total > 0}
			{m.identity_access_scim_enforce_disables_users({
				users: counted(
					pages.unprovisionedUsers.total,
					m.identity_access_scim_count_user_one,
					m.identity_access_scim_count_user_other
				)
			})}
		{/if}
		{m.identity_access_scim_cannot_undo()}
	</p>
	{#if r.enforceBlockers.length > 0}
		{@render blockerList(m.identity_access_scim_cannot_enforce_yet(), r.enforceBlockers)}
	{:else}
		{@render stepStatus(true, m.identity_access_scim_ready_to_enforce())}
	{/if}
{/snippet}

{#snippet groupsSection(r: SCIMConnectionReview)}
	<section class="paper" aria-labelledby="scim-groups-title">
		<h2 id="scim-groups-title" class="text-lg font-semibold">{m.identity_access_groups_tab()}</h2>
		{@render warnings(r.warnings)}

		{#if pages.unboundReferencedGroups.total > 0}
			<div class="flex flex-col gap-2">
				<h3 class="text-sm font-semibold">
					{m.identity_access_scim_referenced_not_pushed_heading({
						count: pages.unboundReferencedGroups.total
					})}
				</h3>
				<p class="text-muted-content text-xs font-light">
					{m.identity_access_scim_push_or_remove({ provider: providerName })}
				</p>
				{@render groupList('unboundReferencedGroups', '')}
			</div>
		{/if}

		<div class="flex flex-col gap-2">
			<h3 class="text-sm font-semibold">
				{m.identity_access_scim_pushed_groups_heading({ count: pages.boundGroups.total })}
			</h3>
			<p class="text-muted-content text-xs font-light">
				{m.identity_access_scim_pushed_help({ provider: providerName })}
			</p>
			{@render groupList(
				'boundGroups',
				m.identity_access_scim_no_groups_from({ provider: providerName })
			)}
		</div>

		{#if pages.unreferencedGroups.total > 0}
			<div class="flex flex-col gap-2">
				<h3 class="text-sm font-semibold">
					{m.identity_access_scim_unreferenced_heading({ count: pages.unreferencedGroups.total })}
				</h3>
				<p class="text-muted-content text-xs font-light">
					{m.identity_access_scim_unreferenced_help()}
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
		<h2 id="scim-users-title" class="text-lg font-semibold">{m.identity_access_users_tab()}</h2>

		<div class="flex flex-col gap-2">
			<h3 class="text-sm font-semibold">
				{m.identity_access_scim_provisioned_heading({ count: pages.provisionedUsers.total })}
			</h3>
			{@render userList(
				'provisionedUsers',
				m.identity_access_scim_no_users_yet({ provider: providerName })
			)}
		</div>

		{#if pages.unprovisionedUsers.total > 0}
			<div class="flex flex-col gap-2">
				<h3 class="text-sm font-semibold">
					{m.identity_access_scim_not_provisioned_heading({
						count: pages.unprovisionedUsers.total
					})}
				</h3>
				<p class="text-muted-content text-xs font-light">
					{m.identity_access_scim_unprovisioned_help({ provider: providerName })}
				</p>
				{@render userList('unprovisionedUsers', '')}
			</div>
		{/if}
	</section>
{/snippet}

{#snippet activitySection(r: SCIMConnectionReview)}
	<section class="paper" aria-labelledby="scim-activity-title">
		<div class="flex flex-col gap-1">
			<h2 id="scim-activity-title" class="text-lg font-semibold">
				{m.identity_access_scim_activity()}
			</h2>
			<p class="text-muted-content text-xs font-light">
				{m.identity_access_scim_activity_help({ provider: providerName })}
			</p>
		</div>
		<dl class="grid grid-cols-1 gap-4 text-sm md:grid-cols-2">
			<div class="flex flex-col gap-1">
				<dt class="text-muted-content text-xs">{m.identity_access_scim_last_request()}</dt>
				<dd>{timeAgo(r.activity.lastRequestAt)}</dd>
			</div>
			<div class="flex flex-col gap-1">
				<dt class="text-muted-content text-xs">{m.identity_access_scim_last_success()}</dt>
				<dd>{timeAgo(r.activity.lastSuccessAt)}</dd>
			</div>
		</dl>

		<div class="flex flex-col gap-2">
			<h3 class="text-sm font-semibold">{m.identity_access_scim_recent_failures()}</h3>
			{#if pages.failures.total === 0}
				<p class="text-muted-content text-sm font-light">
					{m.identity_access_scim_no_recent_failures()}
				</p>
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
			<h2 id="scim-enable-title" class="text-lg font-semibold">
				{m.identity_access_scim_move_title({ provider: providerName })}
			</h2>
			<p class="text-muted-content text-sm font-light">
				{m.identity_access_scim_move_body({ provider: providerName })}
			</p>
			<p class="text-muted-content text-sm font-light">
				{m.identity_access_scim_move_still_signin({ provider: providerName })}
			</p>
		</div>

		{#if p.blockers.length > 0}
			{@render blockerList(m.identity_access_scim_cannot_enable_yet(), p.blockers)}
		{/if}

		{#each p.duplicateGroupNames as duplicate (duplicate.name)}
			<div class="flex flex-col gap-2">
				<h3 class="text-sm font-semibold">
					{m.identity_access_scim_named_groups({
						name: duplicate.name,
						count: duplicate.groups.length
					})}
				</h3>
				{@render groupItems(duplicate.groups)}
			</div>
		{/each}

		<div class="flex items-center justify-end gap-2">
			{#if !canEnable}
				<p class="text-muted-content text-xs font-light">{m.identity_access_scim_owner_only()}</p>
			{/if}
			<button
				class="btn btn-primary"
				disabled={!canEnable || loading || p.blockers.length > 0}
				onclick={() => (confirmEnable = true)}
			>
				{m.identity_access_scim_enable()}
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
		{m.identity_access_scim_delete_unreferenced()}
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
						<!-- A missing group's message already lists its references. -->
						{#if warning.type !== 'missingGroup' && warning.references?.length}
							<span class="text-muted-content text-xs">
								{m.identity_access_scim_referenced_by({
									references: warning.references.map(describeGroupReference).join('; ')
								})}
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
		<!-- Pushed groups are listed without their references, which are too much detail there. -->
		{@render groupItems(pages[list].items, list !== 'boundGroups')}
		{@render pager(list)}
	{/if}
{/snippet}

{#snippet groupItems(groups: SCIMSetupGroup[], showReferences = true)}
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
							{m.identity_access_scim_open_in({ provider: providerName })}
						</a>
					{/if}
				</div>
				{#if showReferences && group.references?.length}
					<p class="text-muted-content text-xs font-light">
						{m.identity_access_scim_referenced_by_inline({
							references: group.references.map(describeGroupReference).join('; ')
						})}
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
						<span class="pill-warning">{m.core_status_disabled()}</span>
					{:else if user.scimID && !user.active}
						<span class="pill-warning">{m.identity_access_scim_deactivated()}</span>
					{/if}
					{#if !user.scimID && !user.signedIn}
						<span class="text-muted-content text-xs"
							>{m.identity_access_scim_never_signed_in()}</span
						>
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
	title={m.identity_access_scim_enable()}
	msg={m.identity_access_scim_enable_confirm({ provider: providerName })}
	note={enableNote}
	submitText={m.identity_access_scim_enable()}
	{loading}
	onsuccess={handleEnable}
	oncancel={() => (confirmEnable = false)}
/>

<Confirm
	show={confirmDeleteGroups}
	title={m.identity_access_scim_delete_unreferenced()}
	msg={m.identity_access_scim_delete_groups_msg({
		groups: counted(
			pages.unreferencedGroups.total,
			m.identity_access_scim_count_unreferenced_group_one,
			m.identity_access_scim_count_unreferenced_group_other
		)
	})}
	note={m.identity_access_scim_delete_groups_note({ provider: providerName })}
	submitText={m.identity_access_scim_delete_groups()}
	{loading}
	onsuccess={handleDeleteUnreferencedGroups}
	oncancel={() => (confirmDeleteGroups = false)}
/>

<Confirm
	show={confirmEnforce}
	title={m.identity_access_scim_enforce_title()}
	msg={m.identity_access_scim_enforce_confirm({ provider: providerName })}
	note={enforceNote}
	submitText={m.identity_access_scim_enforce_title()}
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
					{m.identity_access_scim_copy_token_now({ provider: providerName })}
				</span>
			</div>
			<div class="flex min-w-0 flex-col gap-1">
				<span class="text-muted-content text-xs">{m.identity_access_scim_base_url()}</span>
				{@render copyableValue(issuedToken.baseURL, m.identity_access_scim_copy_base_url())}
			</div>
			<div class="flex min-w-0 flex-col gap-1">
				<span class="text-muted-content text-xs">{m.identity_access_scim_bearer_token()}</span>
				{@render copyableValue(issuedToken.token, m.identity_access_scim_copy_token())}
			</div>
			<div class="flex justify-end">
				<button class="btn btn-primary" onclick={() => tokenDialog?.close()}
					>{m.vmcps_done()}</button
				>
			</div>
		</div>
	{/if}
</ResponsiveDialog>

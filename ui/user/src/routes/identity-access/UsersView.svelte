<script lang="ts">
	import { page } from '$app/state';
	import { tooltip } from '$lib/actions/tooltip.svelte.js';
	import Confirm from '$lib/components/Confirm.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import InfoTooltip from '$lib/components/InfoTooltip.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Search from '$lib/components/Search.svelte';
	import UserLimitNotice from '$lib/components/admin/license/UserLimitNotice.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { COMMUNITY_ENTITLEMENT, PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { isAbortError } from '$lib/errors';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		AdminService,
		UserService,
		Group,
		Role,
		type LocalAuthUser,
		type OrgUser
	} from '$lib/services';
	import { userRoleOptions } from '$lib/services/admin/constants';
	import type { OrgUserStatus } from '$lib/services/user/types';
	import { profile, version } from '$lib/stores';
	import { clearProductAnalyticsConsentDeferral } from '$lib/stores/productTelemetryConsent.svelte';
	import { formatTimeAgo } from '$lib/time';
	import { replaceState } from '$lib/url';
	import {
		clearUrlParams,
		getTableUrlParamsFilters,
		getTableUrlParamsSort,
		setSortUrlParams,
		setFilterUrlParams
	} from '$lib/url.js';
	import { getUserRoleLabel, isClaudeTagUser, validateVersionUserLimit } from '$lib/utils';
	import CreateLocalUserDialog from './CreateLocalUserDialog.svelte';
	import CurrentAccessDialog from './CurrentAccessDialog.svelte';
	import ResetPasswordDialog from './ResetPasswordDialog.svelte';
	import { Handshake, Info, KeyRound, ShieldAlert } from '@lucide/svelte';
	import { debounce } from 'es-toolkit';
	import { untrack } from 'svelte';
	import { fade } from 'svelte/transition';

	interface Props {
		users: OrgUser[];
		localAuthProviderAvailable?: boolean;
		providerDiscoveryComplete?: boolean;
	}

	let {
		users: initialUsers,
		localAuthProviderAvailable = false,
		providerDiscoveryComplete = true
	}: Props = $props();
	let users = $state<OrgUser[]>(untrack(() => initialUsers));
	let localAuthUsers = $state<LocalAuthUser[]>([]);
	let localAuthUsersLoading = $state(untrack(() => localAuthProviderAvailable));
	let localAuthUsersFailed = $state(false);
	let localAuthUsersLoaded = $state(false);
	const deletionReady = $derived(
		providerDiscoveryComplete &&
			(!localAuthProviderAvailable ||
				(localAuthUsersLoaded && !localAuthUsersLoading && !localAuthUsersFailed))
	);
	let query = $derived(page.url.searchParams.get('query') ?? '');
	let urlFilters = $derived(getTableUrlParamsFilters());
	let initSort = $derived(getTableUrlParamsSort({ property: 'created', order: 'desc' }));

	const statusLabels: Record<OrgUserStatus, string> = {
		active: m.identity_access_users_status_active(),
		disabled: m.core_status_disabled(),
		deleted: m.identity_access_users_status_deleted()
	};
	const disabledReasonLabels: Record<string, string> = {
		scim_inactive: m.identity_access_users_status_scim_inactive(),
		scim_unprovisioned: m.identity_access_users_status_scim_unprovisioned()
	};
	// Why a user cannot be deleted in Obot, as the server refuses it: their identity provider still
	// provisions them through SCIM. Once it deactivates them, they can be deleted.
	const PRIVILEGED_ROLES = Role.OWNER | Role.AUDITOR;
	const STILL_PROVISIONED_MESSAGE = m.identity_access_users_still_provisioned();
	// Why a Claude Tag user's role can't be updated and it can't be deleted: its Claude environment owns it,
	// and the server refuses both.
	const CLAUDE_TAG_MESSAGE = m.identity_access_users_claude_tag_managed();

	function mapOrgUser(user: OrgUser, localAuthUser?: LocalAuthUser) {
		return {
			...user,
			orgUserId: user.id,
			localAuthUserId: localAuthUser?.id,
			hasOrgUser: true,
			requirePasswordChange: localAuthUser?.requirePasswordChange ?? false,
			lifecycleStatus: (user.status ?? 'active') as OrgUserStatus,
			status: statusLabels[user.status ?? 'active'],
			disabledReasonLabel: user.disabledReason
				? (disabledReasonLabels[user.disabledReason] ?? user.disabledReason)
				: undefined,
			assignedRole: user.role,
			name: getUserDisplayName(user),
			role: getUserRoleLabel(user.role).split(','),
			effectiveRole: getUserRoleLabel(user.effectiveRole).split(','),
			roleId: user.role & ~Role.AUDITOR,
			auditor: Boolean(user.role & Role.AUDITOR),
			// Only an Owner can enable a user with any of these roles, from their own role or a group.
			privileged: (user.effectiveRole & PRIVILEGED_ROLES) !== 0,
			claudeTag: isClaudeTagUser(user)
		};
	}

	function mapLocalOnlyUser(local: LocalAuthUser) {
		return {
			id: `local-auth:${local.id}`,
			email: local.email,
			username: local.email,
			created: local.created,
			iconURL: '',
			explicitRole: false,
			internal: false,
			groups: [] as string[],
			authProviderGroups: undefined as string[] | undefined,
			orgUserId: undefined as string | undefined,
			localAuthUserId: local.id,
			hasOrgUser: false,
			requirePasswordChange: local.requirePasswordChange,
			lifecycleStatus: undefined as OrgUserStatus | undefined,
			status: m.identity_access_users_status_created(),
			disabledReasonLabel: undefined as string | undefined,
			assignedRole: 0,
			name: '',
			role: [] as string[],
			effectiveRole: [] as string[],
			roleId: Role.BASIC,
			auditor: false,
			privileged: false,
			claudeTag: false,
			managementSource: undefined as OrgUser['managementSource'],
			lastActiveDay: undefined as string | undefined,
			displayName: undefined as string | undefined,
			deletedAt: undefined as string | undefined,
			originalEmail: undefined as string | undefined,
			originalUsername: undefined as string | undefined,
			disabledAt: undefined as string | undefined,
			disabledReason: undefined as OrgUser['disabledReason']
		};
	}

	function rowsForLocalAuth(orgUsers: OrgUser[], localUsers: LocalAuthUser[]) {
		const byId = new Map(orgUsers.map((user) => [user.id, user]));
		const matched: Record<string, true> = {};
		const rows = localUsers.map((local) => {
			const org = local.orgUserId ? byId.get(local.orgUserId) : undefined;
			if (!org) return mapLocalOnlyUser(local);
			matched[org.id] = true;
			return mapOrgUser(org, local);
		});
		for (const user of orgUsers) {
			if (!matched[user.id]) rows.push(mapOrgUser(user));
		}
		return rows;
	}

	const tableData = $derived.by(() => {
		const rows = localAuthProviderAvailable
			? rowsForLocalAuth(users, localAuthUsers)
			: users.map((user) => mapOrgUser(user));
		const q = query.toLowerCase();
		return rows.filter(
			(user) => user.name.toLowerCase().includes(q) || user.email?.toLowerCase().includes(q)
		);
	});

	type TableItem = (typeof tableData)[0];

	let currentAccessDialog = $state<ReturnType<typeof CurrentAccessDialog>>();
	let updateRoleDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let updatingRole = $state<TableItem>();
	let deletingUser = $state<TableItem>();
	let enablingUser = $state<TableItem>();
	let confirmHandoffToUser = $state<TableItem>();
	let confirmAuditorAdditionToUser = $state<TableItem>();
	let loading = $state(false);
	let roleUpdateError = $state('');
	let createUserDialog = $state<ReturnType<typeof CreateLocalUserDialog>>();
	let resetPasswordDialog = $state<ReturnType<typeof ResetPasswordDialog>>();
	let roleOptions = $derived([
		...(profile.current.groups.includes(Group.OWNER)
			? [{ label: m.core_role_owner(), id: Role.OWNER }]
			: []),
		{ label: m.core_role_admin(), id: Role.ADMIN },
		{ label: m.identity_access_roles_power_user_plus_short(), id: Role.POWERUSER_PLUS },
		{ label: m.core_role_power_user(), id: Role.POWERUSER },
		{ label: m.core_role_standard_user(), id: Role.BASIC }
	]);
	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());
	const isNearUserLimit = $derived(validateVersionUserLimit(version.current));

	function closeUpdateRoleDialog() {
		updateRoleDialog?.close();
		updatingRole = undefined;
		roleUpdateError = '';
	}

	function getErrorMessage(error: unknown): string {
		if (error instanceof Error && error.message) {
			return error.message;
		}
		return m.identity_access_users_update_role_failed();
	}

	let listedUsersRequest: AbortController | undefined;

	function startListedUsersRequest(): AbortSignal {
		listedUsersRequest?.abort();
		const controller = new AbortController();
		listedUsersRequest = controller;
		return controller.signal;
	}

	function listedUsersRequestCancelled(signal: AbortSignal, error?: unknown) {
		return signal.aborted || isAbortError(error);
	}

	async function reloadListedUsers() {
		const signal = startListedUsersRequest();
		if (localAuthProviderAvailable) {
			localAuthUsersLoading = true;
			localAuthUsersFailed = false;
		}
		let nextUsers: OrgUser[];
		try {
			nextUsers = await UserService.listUsers({ signal });
		} catch (error) {
			if (listedUsersRequestCancelled(signal, error)) return;
			if (localAuthProviderAvailable) {
				localAuthUsersFailed = true;
				localAuthUsersLoading = false;
			}
			throw error;
		}
		if (signal.aborted) return;
		let nextLocalUsers = localAuthUsers;
		if (localAuthProviderAvailable) {
			try {
				nextLocalUsers = await AdminService.listLocalAuthUsers({ signal });
			} catch (error) {
				if (listedUsersRequestCancelled(signal, error)) return;
				localAuthUsersFailed = true;
				throw error;
			} finally {
				if (!signal.aborted) localAuthUsersLoading = false;
			}
		}
		if (signal.aborted) return;
		users = nextUsers;
		if (localAuthProviderAvailable) {
			localAuthUsers = nextLocalUsers;
			localAuthUsersLoaded = true;
			localAuthUsersFailed = false;
		}
	}

	async function refreshAfterLocalAuthSave() {
		try {
			await reloadListedUsers();
		} catch {
			// The list error state and notification will show refresh failure.
		}
	}

	async function loadLocalAuthUsers() {
		const signal = startListedUsersRequest();
		localAuthUsersLoading = true;
		localAuthUsersFailed = false;
		try {
			const items = await AdminService.listLocalAuthUsers({ signal });
			if (signal.aborted) return;
			localAuthUsers = items;
			localAuthUsersLoaded = true;
			return items;
		} catch (error) {
			if (listedUsersRequestCancelled(signal, error)) return;
			localAuthUsersFailed = true;
		} finally {
			if (!signal.aborted) localAuthUsersLoading = false;
		}
	}

	$effect(() => {
		if (!localAuthProviderAvailable) {
			listedUsersRequest?.abort();
			listedUsersRequest = undefined;
			localAuthUsers = [];
			localAuthUsersLoaded = false;
			return () => {
				listedUsersRequest?.abort();
			};
		}

		loadLocalAuthUsers();

		return () => {
			listedUsersRequest?.abort();
		};
	});

	export function openCreateUser() {
		createUserDialog?.open();
	}

	function isOwnerActionRestricted(user: TableItem) {
		return (
			user.hasOrgUser &&
			user.groups.includes(Group.OWNER) &&
			!profile.current.groups.includes(Group.OWNER)
		);
	}

	function openResetPassword(user: TableItem) {
		if (!user.localAuthUserId || !user.email || isPasswordResetRestricted(user)) return;
		resetPasswordDialog?.open({ id: user.localAuthUserId, email: user.email });
	}

	function isPasswordResetRestricted(user: TableItem) {
		return (
			isOwnerActionRestricted(user) ||
			(user.hasOrgUser && user.auditor && !profile.current.groups.includes(Group.OWNER))
		);
	}

	async function updateUserRole(
		userID: string,
		role: number,
		refreshUsers = true
	): Promise<boolean> {
		loading = true;
		roleUpdateError = '';
		try {
			await AdminService.updateUserRole(userID, role);
			if (refreshUsers) {
				await reloadListedUsers();
			}
			if (profile.current.id === userID) {
				// update with the role change
				profile.current = await UserService.getProfile();
			}
			closeUpdateRoleDialog();
			return true;
		} catch (error) {
			roleUpdateError = getErrorMessage(error);
			updateRoleDialog?.open();
			return false;
		} finally {
			loading = false;
		}
	}

	function composeRole(roleId: number, auditor: boolean): number {
		return auditor ? roleId | Role.AUDITOR : roleId;
	}

	function getUserDisplayName(user: OrgUser): string {
		let display =
			user?.displayName ??
			user?.originalUsername ??
			user?.originalEmail ??
			user?.username ??
			user?.email ??
			m.core_unknown_user();

		if (user?.deletedAt) {
			display = m.identity_access_name_deleted({ name: display });
		}

		return display;
	}

	const updateQuery = debounce((value: string) => {
		query = value;

		if (value) {
			page.url.searchParams.set('query', value);
		} else {
			page.url.searchParams.delete('query');
		}

		replaceState(page.url, { query });
	}, 100);

	const duration = PAGE_TRANSITION_DURATION;
	const auditorReadonlyAdminRoles = [Role.BASIC, Role.POWERUSER, Role.POWERUSER_PLUS];

	const hasValidLicense = $derived(Boolean(version.current.enterprise));
	const isCommunityEdition = $derived(
		version.current.licenseEntitlements?.includes(COMMUNITY_ENTITLEMENT) ?? false
	);
</script>

<div class="mb-4" in:fade={{ duration }}>
	<div class="flex flex-col gap-8">
		<div class="flex flex-col gap-2">
			{#if isNearUserLimit}
				<UserLimitNotice />
			{/if}

			{#if version.current.userLimit}
				<section class="flex items-center justify-end gap-2 text-muted-content">
					{#if localAuthProviderAvailable}
						<InfoTooltip
							placement="bottom-start"
							text={m.identity_access_users_user_count_tooltip()}
						/>
					{/if}
					<p class="text-sm">{m.identity_access_users_user_limits()}</p>
					{#if !hasValidLicense || isCommunityEdition}
						<p class="text-sm">{version.current.userCount} / {version.current.userLimit}</p>
					{:else}
						<p class="text-sm text-muted-content">-</p>
					{/if}
				</section>
			{/if}
			<Search
				value={query}
				class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
				onChange={updateQuery}
				placeholder={m.identity_access_users_search_placeholder()}
			/>
			{#if localAuthProviderAvailable && localAuthUsersFailed}
				<div
					role="alert"
					class="flex items-center justify-between gap-3 notification-error text-xs"
				>
					<span>{m.identity_access_users_local_accounts_load_failed()}</span>
					<button class="btn btn-sm btn-error" onclick={refreshAfterLocalAuthSave}>
						{m.identity_access_users_local_accounts_retry()}
					</button>
				</div>
			{/if}

			<Table
				data={tableData}
				fields={['email', 'name', 'status', 'role', 'effectiveRole', 'lastActiveDay', 'created']}
				filterable={['name', 'email', 'status', 'role', 'effectiveRole']}
				filters={urlFilters}
				onFilter={setFilterUrlParams}
				onClearAllFilters={clearUrlParams}
				sortable={['name', 'email', 'status', 'role', 'effectiveRole', 'lastActiveDay', 'created']}
				headers={[
					{
						title: m.core_status(),
						property: 'status',
						tooltip: m.identity_access_users_status_disabled_tooltip()
					},
					{ title: m.core_name(), property: 'name' },
					{ title: m.identity_access_col_email(), property: 'email' },
					{ title: m.identity_access_users_col_assigned_role(), property: 'role' },
					{
						title: m.identity_access_users_col_actual_role(),
						property: 'effectiveRole',
						tooltip: m.identity_access_users_col_actual_role_tooltip()
					},
					{ title: m.identity_access_users_col_last_active(), property: 'lastActiveDay' },
					{ title: m.core_col_created(), property: 'created' }
				]}
				{initSort}
				onSort={setSortUrlParams}
			>
				{#snippet onRenderColumn(property, d)}
					{#if property === 'status'}
						<div class="flex items-center gap-2">
							{#if !d.hasOrgUser}
								<span class="badge badge-sm whitespace-nowrap badge-ghost">
									{m.identity_access_users_status_created()}
								</span>
							{:else}
								<div class="flex flex-col gap-0.5">
									<div class="flex items-center gap-1">
										<span
											class={[
												'badge badge-sm whitespace-nowrap',
												d.lifecycleStatus === 'active' && 'badge-ghost',
												d.lifecycleStatus === 'disabled' && 'badge-warning badge-soft',
												d.lifecycleStatus === 'deleted' && 'badge-error badge-soft'
											]}
										>
											{d.status}
										</span>
										{#if d.managementSource === 'scim'}
											<span
												class="badge badge-ghost badge-xs"
												use:tooltip={m.identity_access_users_scim_managed_tooltip()}
											>
												{m.identity_access_scim_tab()}
											</span>
										{/if}
									</div>
									{#if d.disabledReasonLabel}
										<span class="text-xs text-muted-content">{d.disabledReasonLabel}</span>
									{/if}
								</div>
							{/if}
							{#if d.requirePasswordChange}
								<InfoTooltip
									text={m.identity_access_users_requires_password_change()}
									classes={{ icon: 'text-warning' }}
									icon={KeyRound}
								/>
							{/if}
						</div>
					{:else if property === 'role'}
						{#if !d.hasOrgUser}
							-
						{:else}
							<div class="flex items-center gap-1">
								{d.role}
								{#if d.explicitRole}
									<div use:tooltip={m.identity_access_users_explicit_role()}>
										<ShieldAlert class="size-5" />
									</div>
								{/if}
							</div>
						{/if}
					{:else if property === 'effectiveRole'}
						{#if !d.hasOrgUser}
							-
						{:else}
							<div class="flex items-center gap-1">
								{d.effectiveRole}
							</div>
						{/if}
					{:else if property === 'lastActiveDay' || property === 'created'}
						{d[property as keyof typeof d] ? formatTimeAgo(d[property], 'day').relativeTime : '-'}
					{:else}
						{d[property as keyof typeof d]}
					{/if}
				{/snippet}
				{#snippet actions(d)}
					<DotDotDot>
						<button
							class="menu-button"
							disabled={!d.hasOrgUser}
							onclick={() => {
								if (!d.orgUserId) return;
								currentAccessDialog?.open({
									kind: 'user',
									id: d.orgUserId,
									name: d.name,
									obotGroups: d.groups,
									authProviderGroups: d.authProviderGroups
								});
							}}
						>
							{m.identity_access_view_access_policies()}
						</button>
						{#if !isAdminReadonly}
							<button
								class="menu-button"
								disabled={!d.hasOrgUser ||
									d.claudeTag ||
									(!profile.current.groups.includes(Group.OWNER) &&
										(d.groups.includes(Group.OWNER) || d.explicitRole))}
								use:tooltip={{
									text: d.claudeTag ? CLAUDE_TAG_MESSAGE : undefined,
									placement: 'left',
									classes: ['z-50']
								}}
								onclick={() => {
									if (!d.hasOrgUser) return;
									updatingRole = d;
									updateRoleDialog?.open();
								}}
							>
								{m.identity_access_update_role()}
							</button>
							{#if d.hasOrgUser && d.lifecycleStatus === 'disabled'}
								<button
									class="menu-button"
									disabled={d.privileged && !profile.current.groups.includes(Group.OWNER)}
									onclick={() => (enablingUser = d)}
								>
									{m.identity_access_users_enable()}
								</button>
							{/if}
							{#if d.localAuthUserId}
								<button
									class="menu-button"
									onclick={() => openResetPassword(d)}
									disabled={isPasswordResetRestricted(d)}
								>
									{m.identity_access_auth_providers_local_auth_reset_password()}
								</button>
							{/if}
							{@const stillProvisioned =
								d.hasOrgUser && d.managementSource === 'scim' && d.lifecycleStatus !== 'disabled'}
							<button
								class="menu-button text-error"
								disabled={!deletionReady ||
									(d.hasOrgUser &&
										(d.claudeTag ||
											stillProvisioned ||
											d.explicitRole ||
											isOwnerActionRestricted(d)))}
								use:tooltip={{
									text: d.claudeTag
										? CLAUDE_TAG_MESSAGE
										: stillProvisioned
											? STILL_PROVISIONED_MESSAGE
											: undefined,
									placement: 'left',
									classes: ['z-50']
								}}
								onclick={() => {
									if (isOwnerActionRestricted(d)) return;
									deletingUser = d;
								}}
							>
								{m.identity_access_users_delete_user()}
							</button>
						{/if}
					</DotDotDot>
				{/snippet}
			</Table>
		</div>
	</div>
</div>

<CurrentAccessDialog bind:this={currentAccessDialog} />

<Confirm
	msg={m.identity_access_users_delete_user_confirm({ email: `${deletingUser?.email}` })}
	show={Boolean(deletingUser)}
	{loading}
	onsuccess={async () => {
		if (!deletingUser || !deletionReady || loading) return;
		if (isOwnerActionRestricted(deletingUser)) {
			deletingUser = undefined;
			return;
		}
		const { orgUserId, localAuthUserId } = deletingUser;
		loading = true;
		try {
			let currentLocalAuthUserId = localAuthUserId;
			if (localAuthProviderAvailable) {
				// A local account may have become linked after the dialog opened.
				const accounts = await loadLocalAuthUsers();
				if (!accounts) return;
				currentLocalAuthUserId = accounts.find((account) =>
					localAuthUserId ? account.id === localAuthUserId : account.orgUserId === orgUserId
				)?.id;
			}
			if (currentLocalAuthUserId) {
				await AdminService.deleteLocalAuthUser(currentLocalAuthUserId);
			} else if (orgUserId) {
				await AdminService.deleteUser(orgUserId);
			}
			await reloadListedUsers();
		} catch {
			// The refusal is shown as a notification, and asking again would be refused again.
		} finally {
			loading = false;
			deletingUser = undefined;
		}
	}}
	oncancel={() => (deletingUser = undefined)}
/>

<Confirm
	title={m.identity_access_users_confirm_enable()}
	msg={m.identity_access_users_enable_msg({ email: enablingUser?.email ?? '' })}
	note={m.identity_access_users_enable_note()}
	show={Boolean(enablingUser)}
	{loading}
	type="info"
	submitText={m.chat_enable()}
	onsuccess={async () => {
		if (!enablingUser?.orgUserId) return;
		loading = true;
		try {
			await AdminService.enableUser(enablingUser.orgUserId);
			await reloadListedUsers();
		} catch {
			// The refusal is shown as a notification, and asking again would be refused again.
		} finally {
			loading = false;
			enablingUser = undefined;
		}
	}}
	oncancel={() => (enablingUser = undefined)}
/>

<ResponsiveDialog
	bind:this={updateRoleDialog}
	class="w-full overflow-visible p-4 md:max-w-xl"
	title={m.identity_access_users_update_role_title({ name: `${updatingRole?.name}` })}
>
	{#if updatingRole}
		{@const roleDescriptionMap = userRoleOptions.reduce(
			(acc, role) => {
				acc[role.id] = role.description;
				return acc;
			},
			{} as Record<number, string>
		)}
		<div class="m-4 flex flex-col gap-2 text-sm font-light">
			{#if roleUpdateError}
				<div class="notification-error mb-2 p-3 text-sm font-light">{roleUpdateError}</div>
			{/if}
			{#if updatingRole.explicitRole}
				<div class="notification-info mb-2 p-3 text-sm font-light">
					<div class="flex items-center gap-3">
						<Info class="size-6" />
						<div>{m.identity_access_users_explicit_role()}</div>
					</div>
				</div>
			{/if}
			{#each roleOptions as role (role.id)}
				<label class="flex gap-4">
					<input
						type="radio"
						value={role.id}
						bind:group={updatingRole.roleId}
						disabled={updatingRole.explicitRole}
					/>
					<span class="flex flex-col" class:opacity-50={updatingRole.explicitRole}>
						<p class="w-28 shrink-0 font-semibold">{role.label}</p>
						<p class="text-muted-content">
							{#if role.id === Role.OWNER}
								{m.identity_access_users_owner_description()}
							{:else if role.id === Role.ADMIN}
								{m.identity_access_users_admin_description()}
							{:else}
								{roleDescriptionMap[role.id]}
							{/if}
						</p>
					</span>
				</label>
			{/each}

			{#if profile.current.groups.includes(Group.OWNER)}
				<label class="mt-4 flex gap-4">
					<input type="checkbox" bind:checked={updatingRole.auditor} />
					<span class="flex flex-col">
						<p class="w-28 shrink-0 font-semibold">{m.core_role_auditor()}</p>
						{#if auditorReadonlyAdminRoles.includes(updatingRole.roleId)}
							<p class="text-muted-content">
								{m.identity_access_users_auditor_readonly_description()}
							</p>
						{:else}
							<p class="text-muted-content">
								{m.identity_access_users_auditor_description()}
							</p>
						{/if}
					</span>
				</label>
			{/if}
		</div>
		<div class="flex grow"></div>
		<div class="mt-4 flex flex-col justify-end gap-2 p-4 md:flex-row md:p-0">
			<button class="btn btn-secondary" onclick={() => closeUpdateRoleDialog()}
				>{m.common_cancel()}</button
			>
			<button
				class="btn btn-primary"
				onclick={async () => {
					if (!updatingRole) return;
					roleUpdateError = '';
					const addingAuditor =
						updatingRole.auditor && (updatingRole.assignedRole & Role.AUDITOR) === 0;
					if (profile.current.isBootstrapUser?.() && updatingRole.roleId === Role.OWNER) {
						updateRoleDialog?.close();
						confirmHandoffToUser = updatingRole;
						return;
					}

					if (addingAuditor) {
						updateRoleDialog?.close();
						confirmAuditorAdditionToUser = updatingRole;
						return;
					}

					if (!updatingRole.orgUserId) return;
					updateUserRole(
						updatingRole.orgUserId,
						composeRole(updatingRole.roleId, updatingRole.auditor)
					);
				}}
				disabled={loading}
			>
				{#if loading}
					<Loading class="size-4" />
				{:else}
					{m.core_update()}
				{/if}
			</button>
		</div>
	{/if}
</ResponsiveDialog>

<Confirm
	show={Boolean(confirmHandoffToUser)}
	{loading}
	onsuccess={async () => {
		if (!confirmHandoffToUser?.orgUserId) return;
		const ok = await updateUserRole(
			confirmHandoffToUser.orgUserId,
			composeRole(confirmHandoffToUser.roleId, confirmHandoffToUser.auditor),
			false
		);
		if (!ok) {
			confirmHandoffToUser = undefined;
			return;
		}
		await AdminService.bootstrapLogout();
		clearProductAnalyticsConsentDeferral();
		window.location.href = '/oauth2/sign_out?rd=/admin';
		confirmHandoffToUser = undefined;
	}}
	oncancel={() => (confirmHandoffToUser = undefined)}
	type="info"
	title={m.identity_access_users_confirm_handoff()}
>
	{#snippet msgContent()}
		<div class="flex items-center justify-center gap-2">
			<Handshake class="size-6" />
			<h3 class="text-xl font-semibold">{m.identity_access_users_confirm_handoff()}</h3>
		</div>
	{/snippet}
	{#snippet note()}
		<div class="my-4 flex flex-col gap-4">
			<p>
				{m.identity_access_users_handoff_note()}
			</p>
			<p>{m.identity_access_are_you_sure_continue()}</p>
		</div>
	{/snippet}
</Confirm>

<Confirm
	type="info"
	title={m.identity_access_users_confirm_auditor_title()}
	msg={m.identity_access_users_grant_auditor_msg({
		user: `${confirmAuditorAdditionToUser?.email || confirmAuditorAdditionToUser?.name}`
	})}
	{loading}
	show={Boolean(confirmAuditorAdditionToUser)}
	onsuccess={async () => {
		if (!confirmAuditorAdditionToUser?.orgUserId) return;
		await updateUserRole(
			confirmAuditorAdditionToUser.orgUserId,
			composeRole(confirmAuditorAdditionToUser.roleId, true)
		);
		confirmAuditorAdditionToUser = undefined;
	}}
	oncancel={() => {
		confirmAuditorAdditionToUser = undefined;
		updateRoleDialog?.open();
	}}
>
	{#snippet note()}
		<div class="flex flex-col gap-4">
			<p class="text-left">
				{#if confirmAuditorAdditionToUser && auditorReadonlyAdminRoles.includes(confirmAuditorAdditionToUser.roleId)}
					{m.identity_access_users_auditor_note_standard()}
				{:else}
					{m.identity_access_users_auditor_note()}
				{/if}
			</p>
			<p>
				{m.identity_access_users_grant_confirm_prefix()}
				<b>{confirmAuditorAdditionToUser?.email || confirmAuditorAdditionToUser?.name}</b>
				{m.identity_access_users_grant_confirm_this_role()}
			</p>
		</div>
	{/snippet}
</Confirm>

<CreateLocalUserDialog bind:this={createUserDialog} onSaved={refreshAfterLocalAuthSave} />
<ResetPasswordDialog bind:this={resetPasswordDialog} onSaved={refreshAfterLocalAuthSave} />

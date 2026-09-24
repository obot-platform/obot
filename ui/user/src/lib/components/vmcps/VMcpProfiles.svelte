<script module lang="ts">
	import type {
		AccessControlRuleSubject,
		OrgGroup,
		ToolOverride,
		VMCPProfile,
		VMCPComponentSet
	} from '$lib/services';

	type ProfileResource = {
		id: string;
		toolOverrides: ToolOverride[];
		grant?: VMCPComponentSet;
		initialEnabledTools?: string[];
	};
	type ProfileManifest = {
		allowAllComponents: boolean;
		name: string;
		users: AccessControlRuleSubject[];
		resources: ProfileResource[];
	};
	type Profile = ProfileManifest & { id: string };
</script>

<script lang="ts">
	import { page } from '$app/state';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import Select from '$lib/components/Select.svelte';
	import VMcpProfileToolsOverride from '$lib/components/vmcps/VMcpProfileToolsOverride.svelte';
	import type { VMcpToolFlow } from '$lib/runes/vmcps/vmcpToolFlow.svelte';
	import { UserService, type OrgUser, type VMCP, type VMCPComponent } from '$lib/services';
	import { compositeEffectiveToolNames, duplicateToolNames } from '$lib/services/user/mcp';
	import { vmcpManifest } from '$lib/services/vmcps/utils';
	import { success } from '$lib/stores/success';
	import { resolveSubjects } from '$lib/subjectResolver';
	import { setUrlParamAndUpdateUrl } from '$lib/url';
	import { getUserRoleLabel } from '$lib/utils';
	import IconButton from '../primitives/IconButton.svelte';
	import McpServerIcon from './McpServerIcon.svelte';
	import {
		ArrowLeft,
		ChevronDown,
		ChevronUp,
		Plus,
		Server,
		Split,
		Trash2,
		UsersRound,
		X
	} from '@lucide/svelte';
	import { untrack } from 'svelte';
	import { fly, slide } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		vmcp?: VMCP;
		toolFlow: VMcpToolFlow;
		onUpdated?: (vmcp: VMCP) => void;
		readonly?: boolean;
	}

	let { vmcp, toolFlow, onUpdated, readonly = false }: Props = $props();
	let profiles = $state<Profile[]>([]);
	let draft = $state<ProfileManifest>();
	let editingId = $derived.by(() => {
		const profileParam = page.url.searchParams.get('profile');
		if (!profileParam) {
			return undefined;
		}
		const profileById = profiles.find((profile) => profile.id === profileParam);
		if (profileById) {
			return profileById.id;
		}
		return undefined;
	});
	let loadedProfileId = $state<string>();
	let saving = $state(false);
	let error = $state('');
	let directoryUsers = $state<OrgUser[]>([]);
	let directoryGroups = $state<OrgGroup[]>([]);
	let resolvedGroups = $state<OrgGroup[]>([]);
	let subjectQuery = $state('');
	let subjectSelection = $state<string | number>();
	let expanded = $state<Record<string, boolean>>({});
	let loadedVMcpId = $state<string>();
	let confirmDeleteProfile = $state<{ id: string; name: string }>();
	let confirmDisableGrant = $state<{ id: string; name: string }>();

	const EVERYONE_GROUP: OrgGroup = { id: '*', name: 'All Obot Users' };
	const GROUP_PAGE_SIZE = 50;

	const nameError = $derived(
		error === 'Enter a profile name.' || error === 'A profile with this name already exists.'
	);
	const subjectsError = $derived(error === 'Assign at least one person or group.');
	const componentServers = $derived(vmcp?.components ?? []);
	const assignedSubjectIds = $derived(new Set(draft?.users.map((subject) => subject.id) ?? []));
	const groupsById = $derived(
		new Map([...resolvedGroups, ...directoryGroups].map((group) => [group.id, group]))
	);
	const subjectOptions = $derived.by(() => {
		const query = subjectQuery.trim().toLowerCase();
		const everyoneMatches = !query || EVERYONE_GROUP.name.toLowerCase().includes(query);
		const users = query
			? directoryUsers.filter(
					(user) =>
						(user.displayName ?? '').toLowerCase().includes(query) ||
						(user.email ?? '').toLowerCase().includes(query) ||
						(user.username ?? '').toLowerCase().includes(query)
				)
			: directoryUsers;
		const groups = everyoneMatches ? [EVERYONE_GROUP, ...directoryGroups] : directoryGroups;
		return [...groups, ...users]
			.filter((entry) => !assignedSubjectIds.has(entry.id))
			.map((entry) => ({
				id: entry.id,
				label: directoryName(entry),
				subject: toSubject(entry)
			}));
	});

	let groupsRequest: AbortController | undefined;

	$effect(() => {
		if (!draft) return;
		void loadUsers();
	});

	$effect(() => {
		if (!draft) return;
		const query = subjectQuery;
		const timer = setTimeout(() => loadGroups(query), query ? 500 : 0);
		return () => {
			clearTimeout(timer);
			groupsRequest?.abort();
		};
	});

	$effect(() => {
		if (!vmcp) return;
		const switched = loadedVMcpId !== vmcp.id;
		loadedVMcpId = vmcp.id;
		if (!switched) return;
		profiles = (vmcp.profiles ?? []).map(profileFromManifest);
		draft = undefined;
		loadedProfileId = undefined;
		error = '';
		expanded = {};
		resolvedGroups = [];
		confirmDisableGrant = undefined;
	});

	$effect(() => {
		const subjects = [...profiles.flatMap((profile) => profile.users), ...(draft?.users ?? [])];
		if (!subjects.some((subject) => subject.type !== 'selector')) return;

		const controller = new AbortController();
		resolveSubjects(
			subjects,
			untrack(() => ({
				users: directoryUsers.length > 0 ? directoryUsers : undefined,
				groups: resolvedGroups
			})),
			{ signal: controller.signal }
		)
			.then((resolved) => {
				if (controller.signal.aborted) return;
				directoryUsers = resolved.users;
				rememberResolvedGroups(resolved.groups);
			})
			.catch((err) => {
				if (controller.signal.aborted) return;
				console.error('Failed to resolve identities:', err);
			});

		return () => controller.abort();
	});

	function componentId(component: VMCPComponent) {
		return component.id || component.mcpServerCatalogEntryID || '';
	}

	function componentName(component: VMCPComponent) {
		return (
			component.name ||
			component.catalogEntry?.manifest?.name ||
			componentId(component) ||
			'Unknown'
		);
	}

	function initialTools(component: VMCPComponent): ToolOverride[] {
		if (component.toolOverrides?.length) {
			return component.toolOverrides.map((tool) => ({ ...tool }));
		}
		return (component.catalogEntry?.manifest?.toolPreview ?? []).map((tool) => ({
			name: tool.name,
			description: tool.description,
			enabled: tool.enabled !== false
		}));
	}

	function cloneProfile<T extends ProfileManifest>(profile: T): T {
		return {
			...profile,
			users: profile.users.map((subject) => ({ ...subject })),
			resources: profile.resources.map((resource) => ({
				...resource,
				toolOverrides: resource.toolOverrides.map((tool) => ({ ...tool }))
			}))
		};
	}

	function isResourceGranted(profile: ProfileManifest, resource: ProfileResource) {
		if (profile.allowAllComponents) return true;
		return resource.grant !== undefined;
	}

	function profileFromManifest(manifest: VMCPProfile): Profile {
		const permissions = manifest.vmcpPermissions;
		return {
			allowAllComponents: permissions?.allowAllComponents ?? false,
			id: manifest.name,
			name: manifest.name,
			users: (manifest.subjects ?? []).map((subject) => ({ ...subject })),
			resources: componentServers
				.map((component) => {
					const id = componentId(component);
					const tools = initialTools(component);
					const grant = permissions?.allowedComponents?.[id];
					const effectiveGrant = grant ?? (permissions?.allowAllComponents ? {} : undefined);
					const names = effectiveGrant?.allowedTools;
					const toolOverrides = clampToComponent(
						effectiveGrant && (names == null || names.includes('*'))
							? tools
							: tools.map((tool) => ({ ...tool, enabled: (names ?? []).includes(tool.name) })),
						id
					);
					return {
						id,
						grant,
						initialEnabledTools: [...enabledToolNames(toolOverrides)],
						toolOverrides
					};
				})
				.filter((resource) => resource.id)
		};
	}

	function profileToManifest(target: Profile): VMCPProfile {
		let profile = { ...target };
		if (profile.allowAllComponents && profileIsRefined(profile)) {
			materializeExplicitGrants(profile);
		}
		const allowedComponents: Record<string, VMCPComponentSet> = {};
		if (!profile.allowAllComponents) {
			for (const resource of profile.resources) {
				const grant = componentGrant(resource);
				if (grant) allowedComponents[resource.id] = grant;
			}
		}
		const vmcpPermissions: NonNullable<VMCPProfile['vmcpPermissions']> = {};
		if (profile.allowAllComponents) vmcpPermissions.allowAllComponents = true;
		else vmcpPermissions.allowedComponents = allowedComponents;
		return {
			name: profile.name,
			subjects: profile.users.map((subject) => ({ ...subject })),
			vmcpPermissions
		};
	}

	function toolGrantDiffersFromBaseline(id: string, tools: ToolOverride[]) {
		const baseline = baselineTools(id);
		const current = [...enabledToolNames(tools)];
		const base = [...enabledToolNames(baseline)];
		if (current.length !== base.length) return true;
		return !current.every((name) => base.includes(name));
	}

	function grantIsRestrictive(grant: VMCPComponentSet | undefined) {
		if (!grant) return false;
		const names = grant.allowedTools;
		return Array.isArray(names) && !names.includes('*');
	}

	function profileIsRefined(profile: ProfileManifest) {
		return profile.resources.some(
			(resource) =>
				grantIsRestrictive(resource.grant) ||
				toolGrantDiffersFromBaseline(resource.id, resource.toolOverrides)
		);
	}

	function materializeExplicitGrants(profile: ProfileManifest) {
		profile.allowAllComponents = false;
		profile.resources = profile.resources.map((resource) => {
			if (resource.grant !== undefined) return resource;
			return {
				...resource,
				grant: explicitComponentGrant(resource.id, resource),
				initialEnabledTools: [...enabledToolNames(resource.toolOverrides)]
			};
		});
	}

	function componentGrant(resource: ProfileResource): VMCPComponentSet | undefined {
		if (resource.grant === undefined) return undefined;
		const names = [...enabledToolNames(resource.toolOverrides)];
		if (
			resource.initialEnabledTools &&
			names.length === resource.initialEnabledTools.length &&
			names.every((name, index) => name === resource.initialEnabledTools?.[index])
		) {
			return resource.grant;
		}
		return { allowedTools: names };
	}

	function baselineTools(id: string) {
		const component = componentServers.find((candidate) => componentId(candidate) === id);
		return component ? clampToComponent(initialTools(component), id) : [];
	}

	function grantMatchingTools(id: string, tools: ToolOverride[]): VMCPComponentSet {
		const names = [...enabledToolNames(tools)];
		return { allowedTools: names };
	}

	function explicitComponentGrant(id: string, resource: ProfileResource): VMCPComponentSet {
		if (!toolGrantDiffersFromBaseline(id, resource.toolOverrides)) return {};
		return grantMatchingTools(id, resource.toolOverrides);
	}

	function materializeAllowListExcept(profile: ProfileManifest, excludedId: string) {
		profile.allowAllComponents = false;
		profile.resources = profile.resources.map((resource) => {
			if (resource.id === excludedId) {
				return { ...resource, grant: undefined };
			}
			if (resource.grant !== undefined) return resource;
			return {
				...resource,
				grant: explicitComponentGrant(resource.id, resource),
				initialEnabledTools: [...enabledToolNames(resource.toolOverrides)]
			};
		});
	}

	function hasExistingAllowedTools(resource: ProfileResource) {
		const allowed = componentGrant(resource)?.allowedTools;
		return Array.isArray(allowed) && allowed.length > 0;
	}

	function grantableToolNames(id: string) {
		const overrides = componentServers.find(
			(component) => componentId(component) === id
		)?.toolOverrides;
		if (!overrides?.length) return undefined;
		return new Set(overrides.filter((tool) => tool.enabled).map((tool) => tool.name));
	}

	function clampToComponent(tools: ToolOverride[], id: string) {
		const grantable = grantableToolNames(id);
		if (!grantable) return tools;
		return tools.map((tool) =>
			grantable.has(tool.name) ? tool : { ...tool, enabled: false as const }
		);
	}

	function lockedToolNames(resource: ProfileResource) {
		const grantable = grantableToolNames(resource.id);
		if (!grantable) return undefined;
		return new Set(
			resource.toolOverrides.filter((tool) => !grantable.has(tool.name)).map((tool) => tool.name)
		);
	}

	function disableLockedTools(resource: ProfileResource) {
		const locked = lockedToolNames(resource);
		if (!locked?.size) return resource.toolOverrides;
		let changed = false;
		const next = resource.toolOverrides.map((tool) => {
			if (!locked.has(tool.name) || tool.enabled === false) return tool;
			changed = true;
			return { ...tool, enabled: false as const };
		});
		return changed ? next : resource.toolOverrides;
	}

	$effect(() => {
		if (!draft) return;
		for (const resource of draft.resources) {
			resource.toolOverrides = disableLockedTools(resource);
		}
	});

	function refineAllowAllIfNeeded(profile: ProfileManifest) {
		if (!profile.allowAllComponents || !profileIsRefined(profile)) return;
		materializeExplicitGrants(profile);
	}

	function setAllowAllComponents(enabled: boolean) {
		const profile = draft;
		if (readonly || !profile || profile.allowAllComponents === enabled) return;
		if (!enabled) {
			profile.allowAllComponents = false;
			return;
		}
		profile.resources = profile.resources.map((resource) => {
			const toolOverrides = baselineTools(resource.id);
			return {
				...resource,
				grant: undefined,
				toolOverrides,
				initialEnabledTools: [...enabledToolNames(toolOverrides)]
			};
		});
		profile.allowAllComponents = true;
	}

	function createProfile() {
		if (readonly) return;
		error = '';
		expanded = {};
		draft = {
			allowAllComponents: false,
			name: '',
			users: [],
			resources: componentServers
				.map((component) => {
					const id = componentId(component);
					const toolOverrides = clampToComponent(initialTools(component), id);
					return {
						id,
						toolOverrides,
						initialEnabledTools: [...enabledToolNames(toolOverrides)]
					};
				})
				.filter((resource) => resource.id)
		};
	}

	function editProfile(profile: Profile) {
		error = '';
		expanded = {};
		draft = cloneProfile(profile);
		refineAllowAllIfNeeded(draft);
		loadedProfileId = profile.id;
	}

	function closeEditor() {
		draft = undefined;
		loadedProfileId = undefined;
		error = '';
		expanded = {};
		confirmDisableGrant = undefined;
	}

	$effect(() => {
		const id = editingId;
		if (!id) {
			if (untrack(() => loadedProfileId)) untrack(closeEditor);
			return;
		}
		const match = profiles.find((profile) => profile.id === id);
		if (!match || untrack(() => loadedProfileId) === match.id) return;
		untrack(() => editProfile(match));
	});

	function cancelEditing() {
		setUrlParamAndUpdateUrl(page.url, 'profile', null);
		closeEditor();
	}

	export function leaveEditor() {
		if (!draft) return false;
		cancelEditing();
		return true;
	}

	async function saveProfile() {
		if (readonly || !draft || !vmcp || saving) return;
		const name = draft.name.trim();
		if (!name) {
			error = 'Enter a profile name.';
			return;
		}
		if (profiles.some((profile) => profile.id !== editingId && profile.name.trim() === name)) {
			error = 'A profile with this name already exists.';
			return;
		}
		if (draft.users.length === 0) {
			error = 'Assign at least one person or group.';
			return;
		}

		const profile: Profile = {
			...cloneProfile({ ...draft, name }),
			id: name
		};
		const next = editingId
			? profiles.map((candidate) => (candidate.id === editingId ? profile : candidate))
			: [...profiles, profile];

		if (!(await persistProfiles(next, `${name} saved on ${vmcp.displayName}.`))) {
			error = 'Failed to save this profile.';
			return;
		}
		cancelEditing();
	}

	function promptDelete(id: string, name: string) {
		if (readonly) return;
		confirmDeleteProfile = { id, name };
	}

	async function deleteProfile(id: string) {
		if (readonly || !vmcp || saving) return false;
		const removed = profiles.find((profile) => profile.id === id);
		if (!removed) return false;

		const next = profiles.filter((profile) => profile.id !== id);
		const saved = await persistProfiles(next, `${removed.name} removed from ${vmcp.displayName}.`);
		if (saved && editingId === id) cancelEditing();
		if (saved) confirmDeleteProfile = undefined;
		return saved;
	}

	function componentsWithLocalToolOverrides(components: VMCPComponent[]) {
		return components.map((component) => {
			const id = componentId(component);
			const local = vmcp?.components?.find((candidate) => componentId(candidate) === id);
			if (!local?.toolOverrides?.length) return component;
			return {
				...component,
				toolOverrides: local.toolOverrides.map((tool) => ({ ...tool }))
			};
		});
	}

	async function persistProfiles(next: Profile[], message: string) {
		if (readonly || !vmcp) return false;
		saving = true;
		try {
			const latest = await UserService.getVMCP(vmcp.id);
			const updated = await UserService.updateVMCP(latest.id, {
				...vmcpManifest(latest),
				components: componentsWithLocalToolOverrides(latest.components ?? []),
				profiles: next.map(profileToManifest)
			});
			profiles = next;
			success.add(message);
			onUpdated?.(updated);
			return true;
		} catch {
			return false;
		} finally {
			saving = false;
		}
	}

	function isDirectoryGroup(entry: OrgUser | OrgGroup): entry is OrgGroup {
		return 'name' in entry;
	}

	function directoryName(entry: OrgUser | OrgGroup) {
		return isDirectoryGroup(entry)
			? entry.name
			: (entry.displayName ?? entry.email ?? entry.username ?? entry.id);
	}

	function toSubject(entry: OrgUser | OrgGroup): AccessControlRuleSubject {
		if (!isDirectoryGroup(entry)) return { type: 'user', id: entry.id };
		return { type: entry.id === EVERYONE_GROUP.id ? 'selector' : 'group', id: entry.id };
	}

	function rememberResolvedGroups(groups: OrgGroup[]) {
		if (groups.length === 0) return;
		// eslint-disable-next-line svelte/prefer-svelte-reactivity
		const next = new Map(resolvedGroups.map((group) => [group.id, group]));
		let changed = false;
		for (const group of groups) {
			if (next.get(group.id)?.name === group.name) continue;
			next.set(group.id, group);
			changed = true;
		}
		if (changed) resolvedGroups = [...next.values()];
	}

	function subjectDisplay(subject: AccessControlRuleSubject) {
		if (subject.type === 'obotGroup') {
			return {
				name: `Obot ${subject.id.charAt(0).toUpperCase()}${subject.id.slice(1)}`,
				group: true,
				iconURL: undefined,
				role: undefined
			};
		}
		if (subject.type === 'selector') {
			const name = subject.id === EVERYONE_GROUP.id ? EVERYONE_GROUP.name : subject.id;
			return { name, group: true, iconURL: undefined, role: undefined };
		}
		if (subject.type === 'group') {
			const group = groupsById.get(subject.id);
			return { name: group?.name || subject.id, group: true, iconURL: undefined, role: undefined };
		}
		const user = directoryUsers.find((candidate) => candidate.id === subject.id);
		return {
			name: user ? directoryName(user) : subject.id,
			group: false,
			iconURL: user?.iconURL,
			role: user?.effectiveRole
		};
	}

	function userInitials(source: string) {
		if (!source) return '?';
		const local = source.includes('@') ? source.split('@')[0] : source;
		const parts = local.split(/[.\-\s]/).filter(Boolean);
		if (parts.length === 0) return '?';
		let initials = parts[0].charAt(0).toUpperCase();
		if (parts.length > 1) {
			initials += parts[parts.length - 1].charAt(0).toUpperCase();
		}
		return initials;
	}

	function addSubject(subject: AccessControlRuleSubject) {
		if (readonly) return;
		subjectSelection = undefined;
		if (!draft) return;
		if (draft.users.some((candidate) => candidate.id === subject.id)) return;
		draft.users = [...draft.users, subject];
		if (subjectsError) error = '';
	}

	function removeSubject(id: string) {
		if (readonly || !draft) return;
		draft.users = draft.users.filter((subject) => subject.id !== id);
	}

	async function loadUsers() {
		if (directoryUsers.length > 0) return;
		try {
			directoryUsers = await UserService.listUsers();
		} catch (err) {
			console.error('Failed to load users:', err);
		}
	}

	async function loadGroups(query: string) {
		groupsRequest?.abort();
		const controller = new AbortController();
		groupsRequest = controller;
		try {
			const page = await UserService.listGroups({
				query: query.trim() || undefined,
				limit: GROUP_PAGE_SIZE,
				signal: controller.signal
			});
			if (controller.signal.aborted) return;
			directoryGroups = [...page.items].sort((a, b) => a.name.localeCompare(b.name));
			rememberResolvedGroups(directoryGroups);
		} catch (err) {
			if (controller.signal.aborted) return;
			console.error('Failed to load groups:', err);
			directoryGroups = [];
		}
	}

	function resourceFor(id: string) {
		return draft?.resources.find((resource) => resource.id === id);
	}

	function applyComponentGrant(id: string, enabled: boolean, removeOverrides = false) {
		const profile = draft;
		if (!profile) return;
		if (!enabled) expanded[id] = false;
		if (!enabled && profile.allowAllComponents) {
			materializeAllowListExcept(profile, id);
			if (!removeOverrides) return;
			const resource = profile.resources.find((entry) => entry.id === id);
			if (!resource) return;
			const toolOverrides = baselineTools(id);
			resource.toolOverrides = toolOverrides;
			resource.initialEnabledTools = [...enabledToolNames(toolOverrides)];
			return;
		}
		profile.resources = profile.resources.map((resource) => {
			if (resource.id !== id) return resource;
			if (!enabled) {
				if (!removeOverrides) return { ...resource, grant: undefined };
				const toolOverrides = baselineTools(id);
				return {
					...resource,
					grant: undefined,
					toolOverrides,
					initialEnabledTools: [...enabledToolNames(toolOverrides)]
				};
			}
			const current = resource.toolOverrides.length ? resource.toolOverrides : baselineTools(id);
			const baseline = baselineTools(id);
			const toolOverrides =
				current.every((tool) => tool.enabled === false) &&
				baseline.some((tool) => tool.enabled !== false)
					? baseline
					: current;
			const next = { ...resource, toolOverrides };
			return {
				...next,
				initialEnabledTools: [...enabledToolNames(toolOverrides)],
				grant: explicitComponentGrant(id, next)
			};
		});
	}

	function setComponentGrant(id: string, name: string, enabled: boolean) {
		if (readonly || !draft) return;
		const resource = resourceFor(id);
		if (!resource || isResourceGranted(draft, resource) === enabled) return;
		if (!enabled && hasExistingAllowedTools(resource)) {
			confirmDisableGrant = { id, name };
			return;
		}
		applyComponentGrant(id, enabled);
	}

	function confirmDisableComponent() {
		if (!confirmDisableGrant) return;
		applyComponentGrant(confirmDisableGrant.id, false, true);
		confirmDisableGrant = undefined;
	}

	const effectiveNameDuplicates = $derived(
		duplicateToolNames(
			compositeEffectiveToolNames(
				componentServers.map((component) => ({
					toolPrefix: component.toolPrefix,
					toolOverrides: resourceFor(componentId(component))?.toolOverrides
				}))
			)
		)
	);

	function modifiableTools(resource: ProfileResource) {
		const locked = lockedToolNames(resource);
		if (!locked) return resource.toolOverrides;
		return resource.toolOverrides.filter((tool) => !locked.has(tool.name));
	}

	function enabledToolCount(resource: ProfileResource) {
		return modifiableTools(resource).filter((tool) => tool.enabled !== false).length;
	}

	function getDisplayListText(names: string[]) {
		if (names.length <= 1) return names[0] ?? '';
		const rest = names.slice(0, names.length > 5 ? 4 : -1);
		const last = names.length > 5 ? `${names.length - 4} others` : names.at(-1);
		return `${rest.join(', ')} and ${last}`;
	}

	function profileUsersDisplayText(profile: Profile) {
		return getDisplayListText(profile.users.map((subject) => subjectDisplay(subject).name));
	}

	function enabledToolNames(tools: ToolOverride[]) {
		return new Set(tools.filter((tool) => tool.enabled !== false).map((tool) => tool.name));
	}

	function profileResourcePreviews(profile: Profile) {
		const summaries = profile.resources
			.map((resource) => {
				const component = componentServers.find(
					(candidate) => componentId(candidate) === resource.id
				);
				const enabled = enabledToolCount(resource);
				const total = modifiableTools(resource).length;
				const componentEnabled = enabledToolNames(
					component
						? modifiableTools({
								id: resource.id,
								toolOverrides: clampToComponent(initialTools(component), resource.id)
							})
						: []
				);
				const profileEnabled = enabledToolNames(modifiableTools(resource));
				const granted = isResourceGranted(profile, resource);
				const changed =
					granted &&
					([...profileEnabled].some((name) => !componentEnabled.has(name)) ||
						[...componentEnabled].some((name) => !profileEnabled.has(name)));
				return {
					id: resource.id,
					name: component ? componentName(component) : resource.id,
					icon: component?.catalogEntry?.manifest?.icon,
					enabled,
					total,
					changed,
					granted
				};
			})
			.sort((a, b) => Number(b.changed) - Number(a.changed));

		return {
			items: summaries.slice(0, 4),
			more: Math.max(summaries.length - 4, 0)
		};
	}

	function applyCollectedTools(component: VMCPComponent, config: VMCPComponent) {
		if (!draft) return;
		const id = componentId(component);
		const incoming = (config.toolOverrides ?? []).map((tool) => ({ ...tool }));
		const removed = incoming
			.filter((tool) => tool.removed)
			.map((tool) => ({ ...tool, enabled: false as const }));
		const live = incoming
			.filter((tool) => !tool.removed)
			.map((tool) => {
				const copy = { ...tool };
				delete copy.removed;
				return copy;
			});
		const liveOverrides = clampToComponent(live, id);
		const toolOverrides = [...liveOverrides, ...removed];
		const enabledNames = [...enabledToolNames(liveOverrides)];
		draft.resources = draft.resources.map((resource) =>
			resource.id === id
				? {
						...resource,
						toolOverrides,
						initialEnabledTools: enabledNames,
						grant: { allowedTools: enabledNames }
					}
				: resource
		);
		refineAllowAllIfNeeded(draft);
		expanded[id] = true;
	}

	function refineTools(event: MouseEvent, component: VMCPComponent) {
		event.preventDefault();
		event.stopPropagation();
		if (readonly || !vmcp) return;
		toolFlow.collectComponentTools(component, vmcp, (config) =>
			applyCollectedTools(component, config)
		);
	}

	function refreshProfileTools(component: VMCPComponent) {
		if (readonly || !vmcp) return;
		const resource = resourceFor(componentId(component));
		toolFlow.refreshTools(
			{
				...component,
				toolOverrides: (resource?.toolOverrides ?? component.toolOverrides)?.map((tool) => ({
					...tool
				}))
			},
			vmcp,
			(config) => applyCollectedTools(component, config)
		);
	}
</script>

<div class="p-3 pt-0 @container">
	{#if draft}
		<div class="mx-auto w-full max-w-4xl">
			{@render editCreate()}
		</div>
	{:else}
		<p class="text-muted-content text-sm font-light mt-2 mb-4">
			Profiles let you control which tools are available to different users, groups, and agents.
			Define a set of tools and assign identities to the profile to provide tailored access through
			the same VMCP endpoint.
		</p>
		{#if vmcp}
			{@render actions()}
			{@render list()}
		{:else}
			<div class="flex flex-col items-center justify-center text-center">
				<div
					class="bg-primary/10 text-primary mb-4 flex size-9 items-center justify-center rounded-md"
				>
					<UsersRound class="size-4" />
				</div>
				<h2 class="font-semibold">No profiles yet</h2>
				<p class="text-muted-content mt-1 max-w-sm text-xs">
					In order to create profiles, you must first create a vMCP.
				</p>
			</div>
		{/if}
	{/if}
</div>

<Confirm
	show={Boolean(confirmDisableGrant)}
	onsuccess={confirmDisableComponent}
	oncancel={() => (confirmDisableGrant = undefined)}
	msg="Are you sure you want to disable this server?"
	title="Disable Server"
	submitText="Disable"
	type="info"
>
	{#snippet note()}
		{@const name = confirmDisableGrant?.name ?? 'this server'}
		You currently have tool overrides set for {name}. Disabling this server will also remove these
		overrides.
	{/snippet}
</Confirm>

<Confirm
	show={Boolean(confirmDeleteProfile)}
	onsuccess={() => {
		if (!confirmDeleteProfile) return;
		void deleteProfile(confirmDeleteProfile.id);
	}}
	oncancel={() => (confirmDeleteProfile = undefined)}
	msg=""
	loading={saving}
	title="Confirm Delete"
>
	{#snippet note()}
		Are you sure you want to delete "<b>{confirmDeleteProfile?.name ?? 'this profile'}</b>"? This
		cannot be undone.
	{/snippet}
</Confirm>

{#snippet actions()}
	{#if !draft && profiles.length > 0 && !readonly}
		<div class="md:absolute md:top-3 md:right-3 z-50 pb-4 md:pb-0">
			<button
				class="btn btn-primary"
				onclick={() => {
					createProfile();
				}}
			>
				<Plus class="size-4" /> Create profile
			</button>
		</div>
	{/if}
{/snippet}

{#snippet editCreate()}
	{#if draft}
		<form
			class="flex flex-col gap-3 border-base-300 bg-base-100 dark:bg-base-300 rounded-xl border p-5 shadow-sm"
			onsubmit={(event) => {
				event.preventDefault();
				if (readonly) return;
				void saveProfile();
			}}
		>
			<header class="flex items-start gap-3">
				<IconButton tooltip={{ text: 'Back to profiles' }} onclick={cancelEditing}>
					<ArrowLeft class="size-4" />
				</IconButton>
				<div class="min-w-0 grow">
					<h2 class="text-md font-semibold">
						{editingId ? (readonly ? 'View profile' : 'Edit profile') : 'Create profile'}
					</h2>
					<p class="text-muted-content text-sm">
						A profile defines a set of tools for this vMCP that a user, agent, or group has access
						to.
					</p>
				</div>
				{#if editingId && !readonly}
					<IconButton
						variant="danger"
						disabled={saving}
						tooltip={{ text: `Delete ${draft?.name ?? 'profile'}`, placement: 'bottom' }}
						onclick={() => {
							if (!editingId || !draft) return;
							promptDelete(editingId, draft.name);
						}}
					>
						<Trash2 class="size-4" />
					</IconButton>
				{/if}
			</header>

			<div class="divider my-0"></div>

			<section>
				<label class="flex flex-col gap-0.5" for="profile-name">
					<span class={twMerge('font-light text-sm', nameError && 'text-error')}>Name</span>
					<input
						id="profile-name"
						class={twMerge(
							'input-text-filled text-sm',
							nameError && 'border-error bg-error/20 ring-error ring-1 focus:ring-1'
						)}
						placeholder="ex. Marketing, Engineering, etc."
						autocomplete="off"
						aria-invalid={nameError || undefined}
						bind:value={draft.name}
						oninput={() => (error = '')}
						disabled={readonly}
					/>
				</label>
			</section>

			<section>
				<div class="divider text-sm font-semibold my-3">MCP Servers</div>
				<div class="mb-4">
					<p class="text-muted-content text-sm font-light">
						Further modify the tools available for each MCP server in this profile below.
					</p>
				</div>
				<label
					for="allow-all-components"
					class="mb-4 flex items-center justify-between gap-4 p-4 border border-base-300 dark:border-base-400 rounded-lg"
				>
					<div>
						<p class="text-sm font-semibold">Allow All Access</p>
						<p class="text-xs text-muted-content leading-tight font-light">
							Grants access to all MCP servers and enabled tools applied to the vMCP.
							<br class="hidden md:block" />
							This includes current and any MCP servers added in the future.
						</p>
					</div>
					<input
						id="allow-all-components"
						type="checkbox"
						class="toggle toggle-sm shrink-0"
						checked={draft.allowAllComponents}
						disabled={readonly}
						aria-label="Allow All Access"
						onchange={(event) => setAllowAllComponents(event.currentTarget.checked)}
					/>
				</label>
				{#if !draft.allowAllComponents}
					{#if componentServers.length === 0}
						<div class="text-muted-content rounded-lg p-5 text-center text-sm">
							No MCP servers available.
						</div>
					{:else}
						<div class="flex flex-col gap-1">
							{#each componentServers as component (componentId(component))}
								{@const id = componentId(component)}
								{@const resource = resourceFor(id)}
								{@const name = componentName(component)}
								{@const granted = resource ? isResourceGranted(draft, resource) : false}
								<div
									class="border-base-300 dark:border-base-400 flex min-w-0 grow flex-col overflow-hidden rounded-lg border"
								>
									{#snippet componentIdentity()}
										<div class="flex h-10 shrink-0 items-center">
											<input
												type="checkbox"
												class="toggle toggle-xs relative z-10"
												checked={granted}
												disabled={readonly || !resource}
												aria-label={granted ? `Disable ${name}` : `Enable ${name}`}
												use:tooltip={{ text: granted ? `Disable ${name}` : `Enable ${name}` }}
												onclick={(event) => event.stopPropagation()}
												onchange={(event) => {
													const next = event.currentTarget.checked;
													event.currentTarget.checked = granted;
													setComponentGrant(id, name, next);
												}}
											/>
										</div>
										{#if component.catalogEntry?.manifest?.icon}
											<img src={component.catalogEntry.manifest.icon} alt="" class="size-5" />
										{:else}
											<div class="icon">
												<Server class="size-5" />
											</div>
										{/if}
										<span class="grow font-medium text-sm">
											{name}
										</span>
									{/snippet}
									{#if resource && granted && resource.toolOverrides.length > 0}
										<button
											type="button"
											class="hover:bg-base-200 dark:hover:bg-base-200/60 flex w-full items-center gap-3 py-1 pl-3 pr-1 text-left"
											aria-expanded={Boolean(expanded[id])}
											aria-label={expanded[id] ? 'Collapse' : 'Expand'}
											onclick={() => (expanded[id] = !expanded[id])}
										>
											{@render componentIdentity()}
											<span class="text-muted-content text-xs">
												{enabledToolCount(resource)} of {modifiableTools(resource).length} tools
											</span>
											<span
												class="text-muted-content flex size-8 shrink-0 items-center justify-center"
												aria-hidden="true"
											>
												{#if expanded[id]}
													<ChevronUp class="size-4" />
												{:else}
													<ChevronDown class="size-4" />
												{/if}
											</span>
										</button>
									{:else if resource && granted}
										<button
											type="button"
											class="hover:bg-base-200 dark:hover:bg-base-200/60 flex w-full items-center gap-3 py-1 pl-3 pr-1 text-left disabled:cursor-not-allowed disabled:opacity-50"
											aria-label="Refine tools"
											onclick={(event) => refineTools(event, component)}
											disabled={readonly}
										>
											{@render componentIdentity()}
											<span
												class="text-muted-content flex size-8 shrink-0 items-center justify-center"
												aria-hidden="true"
											>
												<Split class="size-4" />
											</span>
										</button>
									{:else}
										<div
											class={twMerge(
												'flex w-full items-center gap-3 py-1 px-3 h-12 justify-between',
												resource && !granted && 'opacity-50'
											)}
										>
											{@render componentIdentity()}
											<span class="text-muted-content shrink-0 text-xs">
												{resource && !granted ? 'Disabled' : ''}
											</span>
										</div>
									{/if}
									{#if resource && resource.toolOverrides.length > 0 && expanded[id]}
										<div
											in:slide={{ axis: 'y', duration: 150 }}
											class="border-base-300 bg-base-200/35 dark:bg-base-200 flex flex-col border-t p-2"
										>
											<VMcpProfileToolsOverride
												bind:tools={resource.toolOverrides}
												toolPrefix={component.toolPrefix}
												componentId={id}
												lockedTools={lockedToolNames(resource)}
												lockedReason="Disabled on this vMCP."
												onRefresh={readonly ? undefined : () => refreshProfileTools(component)}
												onToolsChange={() => draft && refineAllowAllIfNeeded(draft)}
												{effectiveNameDuplicates}
												{readonly}
											/>
										</div>
									{/if}
								</div>
							{/each}
						</div>
					{/if}
				{/if}
			</section>

			<section>
				<div class="divider text-sm font-semibold my-3">Identities</div>
				<div class="mb-4">
					<p class="text-muted-content text-sm font-light">
						Grant the following users, agents, and groups access to this profile.
					</p>
				</div>
				<Select
					id="profile-subjects"
					options={subjectOptions}
					bind:query={subjectQuery}
					bind:selected={subjectSelection}
					searchInDropdown
					placeholder="Add identities..."
					searchPlaceholder="Search users or groups..."
					class="bg-base-200 shadow-inner!"
					classes={{ root: 'w-full' }}
					invalid={subjectsError}
					onSelect={(option) => addSubject(option.subject)}
					disabled={readonly}
				/>
				{#if draft.users.length === 0}
					<div class="text-muted-content text-center pb-4 pt-3 text-xs italic font-light">
						No people or groups assigned.
					</div>
				{:else}
					<div class="flex flex-col mt-2">
						{#each draft.users as subject, index (subject.id)}
							{@const display = subjectDisplay(subject)}
							<div
								class="p-2 flex justify-between items-center"
								out:fly={{ x: 100, duration: 100 }}
							>
								<div class="flex items-center gap-2">
									{#if display.group}
										<div
											class="bg-base-300 dark:bg-base-200 flex size-8 items-center justify-center rounded-full text-[8px] font-medium text-white"
										>
											<UsersRound class="size-4" />
										</div>
									{:else if display.iconURL}
										<img
											src={display.iconURL}
											alt=""
											class="size-4 rounded-full object-cover"
											referrerpolicy="no-referrer"
										/>
									{:else}
										<div
											class="bg-base-300 dark:bg-base-200 flex size-8 items-center justify-center rounded-full text-sm font-medium text-white"
										>
											{userInitials(display.name)}
										</div>
									{/if}
									<div class="flex flex-col">
										<span class="text-sm font-light">{display.name}</span>
										<span class="text-muted-content text-xs">
											{display.group
												? 'Group'
												: display.role
													? getUserRoleLabel(display.role)
													: 'User'}
										</span>
									</div>
								</div>

								{#if !readonly}
									<IconButton
										class="size-8"
										tooltip={{ text: `Remove ${display.name}` }}
										onclick={() => removeSubject(subject.id)}
										variant="danger"
									>
										<X class="size-4" />
									</IconButton>
								{/if}
							</div>
							{#if index < draft.users.length - 1}
								<div class="divider my-0 after:h-px before:h-px px-2"></div>
							{/if}
						{/each}
					</div>
				{/if}
			</section>
			<p class="text-error text-center text-xs min-h-4" role="alert">
				{#if error}
					{error}
				{/if}
			</p>

			{#if !readonly}
				<footer class="w-full">
					<button type="submit" class="btn btn-sm btn-primary text-xs w-full" disabled={saving}>
						{editingId ? 'Save changes' : 'Create profile'}
					</button>
				</footer>
			{/if}
		</form>
	{/if}
{/snippet}

{#snippet list()}
	{#if profiles.length === 0}
		<div class="flex flex-col items-center justify-center text-center">
			<div
				class="bg-primary/10 text-primary mb-4 flex size-9 items-center justify-center rounded-md"
			>
				<UsersRound class="size-4" />
			</div>
			<h2 class="font-semibold">No profiles yet</h2>
			<p class="text-muted-content mt-1 max-w-sm text-xs">
				{readonly
					? 'No profiles have been created for this vMCP yet.'
					: 'Create a profile to group identities and define the MCP tools available to them.'}
			</p>
			{#if !readonly}
				<button class="btn btn-primary btn-sm mt-5" onclick={createProfile}>
					<Plus class="size-4" />
					Create profile
				</button>
			{/if}
		</div>
	{:else}
		<div class="grid grid-cols-1 @2xl:grid-cols-2 @4xl:grid-cols-3 gap-4">
			{#each profiles as profile (profile.id)}
				{@const resources = profileResourcePreviews(profile)}
				<article
					class="border-base-300 dark:border-base-400 bg-base-100 dark:bg-base-300 dark:hover:border-primary/80 hover:border-primary/80 group relative rounded-xl border p-4 shadow-sm transition"
				>
					<button
						type="button"
						class="absolute inset-0 rounded-xl"
						aria-label={`${readonly ? 'View' : 'Edit'} ${profile.name}`}
						onclick={() => {
							setUrlParamAndUpdateUrl(page.url, 'profile', profile.id);
						}}
					></button>
					<div class="pointer-events-none relative">
						<div class="flex items-start justify-between gap-3">
							<div class="min-w-0">
								<h3 class="truncate font-semibold">{profile.name}</h3>
								{#if profile.users.length > 0}
									<p class="text-muted-content mt-1 flex items-center gap-1.5 text-xs">
										<UsersRound class="size-3 shrink-0" />
										<span class="truncate">{profileUsersDisplayText(profile)}</span>
									</p>
								{/if}
								{#if resources.items.length > 0}
									<ul class="mt-4 flex flex-wrap gap-1">
										{#each resources.items as resource (resource.id)}
											<li
												title={resource.name}
												class={twMerge(
													'bg-base-100 dark:bg-base-300 border-base-300 dark:border-base-400 group-hover:border-primary/40 flex shrink-0 items-center gap-2 rounded-md border pr-2 transition-colors',
													!resource.granted && 'opacity-50'
												)}
											>
												<McpServerIcon
													icon={resource.icon}
													width={12}
													height={12}
													class="size-3"
													classes={{ root: 'rounded-r-none' }}
												/>
												<span
													class={twMerge(
														'text-xs whitespace-nowrap text-muted-content',
														resource.changed && 'text-base-content'
													)}
												>
													{resource.changed
														? `${resource.enabled} of ${resource.total}`
														: resource.granted
															? 'Default'
															: 'Disabled'}
												</span>
											</li>
										{/each}
										{#if resources.more > 0}
											<li
												class="text-muted-content self-center text-xs font-light badge bg-transparent border-base-300 dark:border-base-400"
											>
												+{resources.more} more
											</li>
										{/if}
									</ul>
								{/if}
							</div>
							{#if !readonly}
								<IconButton
									variant="danger"
									class="pointer-events-auto"
									disabled={saving}
									tooltip={{ text: `Delete ${profile.name}`, placement: 'bottom' }}
									onclick={() => promptDelete(profile.id, profile.name)}
								>
									<Trash2 class="size-4" />
								</IconButton>
							{/if}
						</div>
					</div>
				</article>
			{/each}
		</div>
	{/if}
{/snippet}

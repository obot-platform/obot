<script lang="ts">
	import { resolve } from '$app/paths';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Search from '$lib/components/Search.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import { parseErrorContent } from '$lib/errors';
	import {
		AdminService,
		ModelAliasLabels,
		type HostedAgent,
		type Model,
		type ModelAlias,
		type OrgUser,
		UserService,
		type SkillRepository
	} from '$lib/services';
	import type { Skill } from '$lib/services/nanobot/types';
	import { errors, mcpServersAndEntries } from '$lib/stores';
	import { getUserDisplayName } from '$lib/utils';
	import {
		ACCESS_MATCH_REASON_LABEL,
		collectAccessResources,
		EVERYTHING_RESOURCE_ID,
		groupMcpAccessPolicies,
		loadCurrentAccess,
		type AccessPolicyResource,
		type AccessResourceDescription,
		type CurrentAccessSectionKey,
		type CurrentAccessSections,
		type CurrentAccessTarget,
		type MatchedAccessPolicy,
		type MatchedAccessResource
	} from './currentAccess';
	import { untrack } from 'svelte';
	import { SvelteSet } from 'svelte/reactivity';

	type SectionKey = CurrentAccessSectionKey;

	interface Props {
		target?: CurrentAccessTarget;
	}

	let { target }: Props = $props();

	const emptySections = (): CurrentAccessSections => ({
		vmcps: [],
		mcp: [],
		models: [],
		skills: [],
		hostedAgents: []
	});

	const idleLoading = (): Record<SectionKey, boolean> => ({
		vmcps: false,
		mcp: false,
		models: false,
		skills: false,
		hostedAgents: false
	});

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let viewing = $state<CurrentAccessTarget>();
	let sections = $state<CurrentAccessSections>(emptySections());
	let loadingSections = $state<Record<SectionKey, boolean>>(idleLoading());
	let sectionErrors = $state<Partial<Record<SectionKey, string>>>({});
	let loadGeneration = 0;
	let currentTab = $state<SectionKey>('vmcps');
	let resourceQuery = $state('');
	let search = $state<ReturnType<typeof Search>>();

	let models = $state<Model[]>([]);
	let skills = $state<Skill[]>([]);
	let skillRepositories = $state<SkillRepository[]>([]);
	let hostedAgents = $state<HostedAgent[]>([]);
	let mcpOwners = $state<OrgUser[]>([]);
	let loadingResources = $state<Record<SectionKey, boolean>>(idleLoading());
	let loadedPolicySections = new SvelteSet<SectionKey>();
	let loadedResourceSections = new SvelteSet<SectionKey>();
	let resourceLoadFailed = new SvelteSet<SectionKey>();

	export function open(next?: CurrentAccessTarget) {
		viewing = next ?? target;
		currentTab = 'vmcps';
		dialog?.open();
	}

	function resetLoadState() {
		loadGeneration += 1;
		sections = emptySections();
		loadingSections = idleLoading();
		sectionErrors = {};
		loadingResources = idleLoading();
		loadedPolicySections.clear();
		loadedResourceSections.clear();
		resourceLoadFailed.clear();
	}

	async function onOpen() {
		resetLoadState();
		await loadSectionPolicies(currentTab);
	}

	function onClose() {
		resetLoadState();
		viewing = undefined;
		clearResourceQuery();
	}

	async function loadSectionPolicies(section: SectionKey) {
		const current = viewing ?? target;
		if (!current || loadedPolicySections.has(section)) {
			return;
		}

		const generation = loadGeneration;
		loadingSections[section] = true;
		sectionErrors[section] = '';

		try {
			const policies = await loadCurrentAccess(current, section);
			if (generation !== loadGeneration) {
				return;
			}
			sections[section] = policies;
			loadedPolicySections.add(section);
		} catch (error) {
			if (generation !== loadGeneration) {
				return;
			}
			sectionErrors[section] =
				parseErrorContent(error).message || 'Failed to load access policies.';
		} finally {
			if (generation === loadGeneration) {
				loadingSections[section] = false;
			}
		}
	}

	function clearResourceQuery() {
		resourceQuery = '';
		search?.clear();
	}

	$effect(() => {
		const section = currentTab;
		const policies = sections[section];
		const loadingPolicies = loadingSections[section];
		if (
			loadingPolicies ||
			!loadedPolicySections.has(section) ||
			policies.length === 0 ||
			section === 'vmcps'
		) {
			return;
		}

		untrack(() => loadSectionResources(section));
	});

	async function loadSectionResources(section: SectionKey) {
		if (loadedResourceSections.has(section)) {
			return;
		}
		loadedResourceSections.add(section);
		resourceLoadFailed.delete(section);
		loadingResources[section] = true;

		try {
			switch (section) {
				case 'mcp': {
					const hasPowerUserPolicies = sections.mcp.some((policy) => policy.powerUserID);
					await Promise.all([
						mcpServersAndEntries.initialize({ scope: 'admin' }),
						hasPowerUserPolicies
							? UserService.listUsersIncludeDeleted()
									.then((users) => (mcpOwners = users))
									.catch(() => undefined)
							: Promise.resolve()
					]);
					break;
				}
				case 'models':
					models = await AdminService.listModels({ all: true });
					break;
				case 'skills':
					[skills, skillRepositories] = await Promise.all([
						AdminService.listAllSkills(),
						AdminService.listSkillRepositories()
					]);
					break;
				case 'hostedAgents':
					hostedAgents = await AdminService.listHostedAgents({ all: true });
					break;
			}
		} catch (error) {
			loadedResourceSections.delete(section);
			resourceLoadFailed.add(section);
			errors.append(error);
		} finally {
			loadingResources[section] = false;
		}
	}

	const mcpEntriesMap = $derived(
		new Map(mcpServersAndEntries.current.entries.map((entry) => [entry.id, entry]))
	);
	const mcpServersMap = $derived(
		new Map(mcpServersAndEntries.current.servers.map((server) => [server.id, server]))
	);
	const modelsMap = $derived(new Map(models.map((model) => [model.id, model])));
	const skillsMap = $derived(new Map(skills.map((skill) => [skill.id, skill])));
	const skillRepositoriesMap = $derived(
		new Map(skillRepositories.map((repository) => [repository.id, repository]))
	);
	const hostedAgentsMap = $derived(new Map(hostedAgents.map((agent) => [agent.id, agent])));
	const mcpOwnersMap = $derived(new Map(mcpOwners.map((owner) => [owner.id, owner])));

	function describeResource(resource: AccessPolicyResource): AccessResourceDescription {
		switch (resource.type) {
			case 'mcpServerCatalogEntry':
				return {
					name: mcpEntriesMap.get(resource.id)?.manifest?.name || resource.id,
					typeLabel: 'Catalog Entry'
				};
			case 'mcpServer': {
				const server = mcpServersMap.get(resource.id);
				return {
					name: server?.alias || server?.manifest?.name || resource.id,
					typeLabel: 'MCP Server'
				};
			}
			case 'model': {
				if (resource.id.startsWith('obot://')) {
					const alias = resource.id.replace('obot://', '') as ModelAlias;
					return { name: ModelAliasLabels[alias] || alias, typeLabel: 'Model Alias' };
				}
				if (resource.id.endsWith('*')) {
					return { name: resource.id, typeLabel: 'Model Pattern' };
				}
				const model = modelsMap.get(resource.id);
				return { name: model?.displayName || model?.name || resource.id };
			}
			case 'skill':
				return { name: skillsMap.get(resource.id)?.name || resource.id };
			case 'skillRepository':
				return {
					name: skillRepositoriesMap.get(resource.id)?.displayName || resource.id,
					typeLabel: 'Skill Repository'
				};
			case 'hostedAgent':
				return { name: hostedAgentsMap.get(resource.id)?.name || resource.id };
			case 'vmcp':
				return { name: resource.name || resource.id };
			default:
				return { name: resource.id };
		}
	}

	const titleName = $derived(viewing?.name ?? target?.name ?? '');
	const subjectLabel = $derived(viewing?.kind === 'group' ? 'group' : 'user');

	const tabs = [
		{ label: 'vMCPs', value: 'vmcps', noun: 'vMCPs' },
		{ label: 'MCP Servers', value: 'mcp', noun: 'MCP servers' },
		{ label: 'Models', value: 'models', noun: 'models' },
		{ label: 'Skills', value: 'skills', noun: 'skills' },
		{ label: 'Hosted Agents', value: 'hostedAgents', noun: 'hosted agents' }
	] as const satisfies readonly { label: string; value: SectionKey; noun: string }[];

	const currentNoun = $derived(tabs.find((tab) => tab.value === currentTab)?.noun ?? 'resources');
	const currentPolicies = $derived(sections[currentTab]);
	const awaitingCurrentCatalog = $derived(
		currentTab !== 'vmcps' &&
			currentPolicies.length > 0 &&
			!loadedResourceSections.has(currentTab) &&
			!resourceLoadFailed.has(currentTab)
	);
	const currentResources = $derived(
		collectAccessResources(
			currentPolicies,
			describeResource,
			currentTab === 'vmcps' || !loadedResourceSections.has(currentTab)
				? undefined
				: sectionCatalog(currentTab)
		)
	);

	const mcpPolicyGroups = $derived(
		groupMcpAccessPolicies(sections.mcp).map((group) => ({
			...group,
			resources: collectAccessResources(
				group.policies,
				describeResource,
				loadedResourceSections.has('mcp') ? mcpRegistryResources(group.powerUserID) : undefined
			)
		}))
	);

	function sectionCatalog(section: SectionKey): AccessPolicyResource[] {
		switch (section) {
			case 'models':
				return models
					.filter((model) => model.usage === 'llm')
					.map((model) => ({ type: 'model', id: model.id }));
			case 'skills':
				return [
					...skills.map((skill) => ({ type: 'skill' as const, id: skill.id })),
					...skillRepositories.map((repository) => ({
						type: 'skillRepository' as const,
						id: repository.id
					}))
				];
			case 'hostedAgents':
				return hostedAgents.map((agent) => ({ type: 'hostedAgent' as const, id: agent.id }));
			default:
				return [];
		}
	}

	function mcpRegistryResources(powerUserID?: string): AccessPolicyResource[] {
		const entries = mcpServersAndEntries.current.entries.filter((entry) => {
			if (entry.deleted) {
				return false;
			}
			return powerUserID ? entry.powerUserID === powerUserID : !entry.powerUserWorkspaceID;
		});
		const servers = mcpServersAndEntries.current.servers.filter((server) => {
			if (server.deleted) {
				return false;
			}
			return powerUserID
				? Boolean(server.powerUserWorkspaceID) && server.userID === powerUserID
				: !server.powerUserWorkspaceID;
		});

		return [
			...entries.map((entry) => ({ type: 'mcpServerCatalogEntry' as const, id: entry.id })),
			...servers.map((server) => ({ type: 'mcpServer' as const, id: server.id }))
		];
	}

	function mcpGroupLabel(powerUserID?: string): string {
		if (!powerUserID) {
			return '';
		}
		const owner = mcpOwnersMap.get(powerUserID);
		return `${owner ? getUserDisplayName(mcpOwnersMap, powerUserID) : 'Unknown'}'s Registry`;
	}

	function matchesSearch(...parts: (string | undefined)[]): boolean {
		const query = resourceQuery.trim().toLowerCase();
		if (!query) {
			return true;
		}
		return parts.some((part) => part?.toLowerCase().includes(query));
	}

	function resourceMatchesSearch(resource: MatchedAccessResource): boolean {
		return matchesSearch(
			resource.name,
			resource.typeLabel,
			...resource.policies.map((policy) => policy.displayName)
		);
	}

	const everythingLabel: Record<SectionKey, string> = {
		vmcps: 'All vMCPs',
		mcp: 'All MCP servers',
		models: 'All models',
		skills: 'All skills',
		hostedAgents: 'All hosted agents'
	};

	function policiesGrantingEverything(policies: MatchedAccessPolicy[]): MatchedAccessPolicy[] {
		return policies.filter((policy) =>
			policy.resources.some((resource) => resource.id === EVERYTHING_RESOURCE_ID)
		);
	}

	function includesEverythingRow(policies: MatchedAccessPolicy[], label: string): boolean {
		return (
			policies.length > 0 && matchesSearch(label, ...policies.map((policy) => policy.displayName))
		);
	}

	const filteredMcpPolicyGroups = $derived(
		mcpPolicyGroups.flatMap((group) => {
			const label = mcpGroupLabel(group.powerUserID);
			const groupMatches = matchesSearch(label);
			const resources = groupMatches
				? group.resources
				: group.resources.filter(resourceMatchesSearch);
			const unexpandedPolicies = resourceLoadFailed.has('mcp')
				? policiesGrantingEverything(group.policies)
				: [];
			const includeUnexpanded = groupMatches
				? unexpandedPolicies.length > 0
				: includesEverythingRow(unexpandedPolicies, everythingLabel.mcp);
			if (resources.length === 0 && !includeUnexpanded) {
				return [];
			}
			return [
				{
					...group,
					resources,
					unexpandedPolicies: includeUnexpanded ? unexpandedPolicies : []
				}
			];
		})
	);

	const filteredCurrentResources = $derived(currentResources.filter(resourceMatchesSearch));
	const hasResourceQuery = $derived(resourceQuery.trim().length > 0);
	const catalogUnavailable = $derived(resourceLoadFailed.has(currentTab));

	const unexpandedEverythingPolicies = $derived(
		catalogUnavailable && currentTab !== 'mcp' ? policiesGrantingEverything(currentPolicies) : []
	);
	const showUnexpandedEverything = $derived(
		includesEverythingRow(unexpandedEverythingPolicies, everythingLabel[currentTab])
	);
	const wildcardExpansionIncomplete = $derived(
		catalogUnavailable && policiesGrantingEverything(currentPolicies).length > 0
	);
</script>

<ResponsiveDialog
	bind:this={dialog}
	{onOpen}
	{onClose}
	title={titleName ? `${titleName} | Access Policies` : 'Access Policies'}
	class="w-full overflow-hidden md:h-150 md:max-w-4xl"
	classes={{ header: 'p-4 md:pb-0', content: 'min-h-inherit p-0' }}
>
	<div class="default-scrollbar-thin flex grow flex-col gap-4 overflow-y-auto px-4 pt-0 pb-4">
		<p class="text-muted-content text-sm font-light mt-4 md:mt-0">
			Resources this {subjectLabel} can access through assigned policies, including those assigned to
			All Obot Users
			{#if viewing?.kind === 'user'}
				and any groups they belong to
			{/if}.
		</p>

		<div class="flex flex-col">
			<div class="tabs tabs-box shadow-inner">
				{#each tabs as tab (tab.value)}
					<button
						class="tab {currentTab === tab.value ? 'tab-active dark:bg-base-300' : ''}"
						onclick={() => {
							currentTab = tab.value;
							clearResourceQuery();
							loadSectionPolicies(tab.value);
						}}
					>
						{tab.label}
					</button>
				{/each}
			</div>
			<div class="mt-2">
				<Search
					bind:this={search}
					compact
					value={resourceQuery}
					placeholder="Search {currentNoun}..."
					onChange={(value) => (resourceQuery = value)}
				/>
			</div>
			{#if loadingSections[currentTab]}
				<Skeleton type="items" count={3} />
			{:else if sectionErrors[currentTab]}
				<div class="notification-error p-3 text-sm font-light" role="alert">
					{sectionErrors[currentTab]}
				</div>
			{:else}
				{@render resourceSection()}
			{/if}
		</div>
	</div>
</ResponsiveDialog>

{#snippet resourceRow(name: string, policies: MatchedAccessPolicy[])}
	<div class="flex flex-col px-2 py-2">
		<span class="text-sm">{name}</span>
		<span class="text-muted-content text-xs font-light">
			<span>Granted by</span>
			{#each policies as policy, index (policy.id)}
				{#if index > 0}<span>,</span>{/if}
				<a class="link link-hover" href={resolve(policy.href)}>{policy.displayName}</a>
				<span>({policy.reasons.map((reason) => ACCESS_MATCH_REASON_LABEL[reason]).join(', ')})</span
				>
			{/each}
		</span>
	</div>
{/snippet}

{#snippet resourceRows(resources: MatchedAccessResource[])}
	{#each resources as resource, index (resource.key)}
		{@render resourceRow(resource.name, resource.policies)}
		{#if index < resources.length - 1}
			<div class="divider my-0.5 h-2 before:h-px after:h-px"></div>
		{/if}
	{/each}
{/snippet}

{#snippet catalogWarning()}
	<div class="notification-alert mb-2 p-3 text-sm font-light" role="status">
		{#if wildcardExpansionIncomplete}
			The catalog could not be loaded, so names may be missing and wildcard grants are not expanded.
		{:else}
			The catalog could not be loaded, so resource names may be missing.
		{/if}
	</div>
{/snippet}

{#snippet unexpandedEverythingRow(
	label: string,
	policies: MatchedAccessPolicy[],
	hasResources: boolean
)}
	{@render resourceRow(label, policies)}
	{#if hasResources}
		<div class="divider my-0.5 h-2 before:h-px after:h-px"></div>
	{/if}
{/snippet}

{#snippet resourceSection()}
	<section class="mt-2">
		{#if currentPolicies.length === 0}
			<p class="text-muted-content px-1 py-2 text-sm font-light italic">
				No policies currently apply to this {subjectLabel}.
			</p>
		{:else if currentTab === 'mcp'}
			{#if loadingResources.mcp || awaitingCurrentCatalog}
				<Skeleton type="items" count={3} />
			{:else}
				{#if catalogUnavailable}
					{@render catalogWarning()}
				{/if}
				{#if filteredMcpPolicyGroups.length === 0}
					<p class="text-muted-content px-1 text-sm font-light">
						{hasResourceQuery
							? `No ${currentNoun} match this search.`
							: 'No MCP servers are granted through this registry.'}
					</p>
				{:else}
					{#each filteredMcpPolicyGroups as group, groupIndex (group.key)}
						{@const label = mcpGroupLabel(group.powerUserID)}
						<section class="flex flex-col" aria-label={label}>
							{#if label}
								<h3 class="text-muted-content px-2 pt-2 text-xs font-semibold uppercase">
									{label}
								</h3>
							{/if}
							{#if group.unexpandedPolicies.length > 0}
								{@render unexpandedEverythingRow(
									everythingLabel.mcp,
									group.unexpandedPolicies,
									group.resources.length > 0
								)}
							{/if}
							{#if group.resources.length === 0 && group.unexpandedPolicies.length === 0}
								<p class="text-muted-content px-2 py-2 text-sm font-light">
									No MCP servers are granted through this registry.
								</p>
							{:else}
								{@render resourceRows(group.resources)}
							{/if}
						</section>
						{#if groupIndex < filteredMcpPolicyGroups.length - 1}
							<div class="divider my-1 h-2 before:h-px after:h-px"></div>
						{/if}
					{/each}
				{/if}
			{/if}
		{:else if loadingResources[currentTab] || awaitingCurrentCatalog}
			<Skeleton type="items" count={3} />
		{:else}
			{#if catalogUnavailable}
				{@render catalogWarning()}
			{/if}
			{#if filteredCurrentResources.length === 0 && !showUnexpandedEverything}
				<p class="text-muted-content px-1 text-sm font-light">
					{hasResourceQuery
						? `No ${currentNoun} match this search.`
						: `The policies that apply to this ${subjectLabel} do not grant access to any ${currentNoun}.`}
				</p>
			{:else}
				{#if showUnexpandedEverything}
					{@render unexpandedEverythingRow(
						everythingLabel[currentTab],
						unexpandedEverythingPolicies,
						filteredCurrentResources.length > 0
					)}
				{/if}
				{@render resourceRows(filteredCurrentResources)}
			{/if}
		{/if}
	</section>
{/snippet}

<script lang="ts">
	import { DEFAULT_MCP_CATALOG_ID, PAGE_TRANSITION_DURATION } from '$lib/constants';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		AdminService,
		type Harness,
		type HostedAgent,
		type HostedAgentManifest,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type Model,
		type ModelProvider,
		type SkillAccessPolicyResource,
		type SkillRepository
	} from '$lib/services';
	import type { Skill } from '$lib/services/nanobot/types';
	import { defaultModelAliases as defaultModelAliasesStore, errors } from '$lib/stores';
	import { goto } from '$lib/url';
	import Confirm from '../Confirm.svelte';
	import Search from '../Search.svelte';
	import IconButton from '../primitives/IconButton.svelte';
	import HostedAgentEnvEditor from './HostedAgentEnvEditor.svelte';
	import HostedAgentQuestionsEditor from './HostedAgentQuestionsEditor.svelte';
	import SearchModels from './SearchModels.svelte';
	import SearchSkills from './SearchSkills.svelte';
	import { Plus, Trash2 } from '@lucide/svelte';
	import { onMount, untrack } from 'svelte';
	import { fly } from 'svelte/transition';

	interface Props {
		hostedAgent?: HostedAgent;
		onCreate?: (hostedAgent: HostedAgent) => void;
		onUpdate?: (hostedAgent: HostedAgent) => void;
		readonly?: boolean;
	}

	let { hostedAgent: initialHostedAgent, onCreate, onUpdate, readonly }: Props = $props();

	const duration = PAGE_TRANSITION_DURATION;

	function emptyAgent(): HostedAgentManifest & { id?: string } {
		return {
			id: undefined,
			name: '',
			description: '',
			icon: '',
			iconDark: '',
			harnessID: '',
			gitRepo: '',
			modelProviders: [],
			models: [],
			mcpServers: [],
			skills: [],
			env: [],
			questions: [],
			allowUserMCPServers: false,
			allowUserSkills: false,
			allowUserModels: false,
			allowUserGitRepo: false,
			maxInstancesPerUser: 0
		};
	}

	let agent = $state(untrack(() => ({ ...emptyAgent(), ...(initialHostedAgent ?? {}) })));

	let saving = $state(false);
	let deleting = $state(false);
	let loadingServices = $state(true);
	let harnesses = $state<Harness[]>([]);
	let modelProviders = $state<ModelProvider[]>([]);
	let mcpEntries = $state<MCPCatalogEntry[]>([]);
	let mcpCatalogServers = $state<MCPCatalogServer[]>([]);
	let models = $state<Model[]>([]);
	let defaultModelAliases = $derived(defaultModelAliasesStore.current);
	let skills = $state<Skill[]>([]);
	let skillRepositories = $state<SkillRepository[]>([]);

	let addModelDialog = $state<ReturnType<typeof SearchModels>>();
	let addSkillDialog = $state<ReturnType<typeof SearchSkills>>();

	let modelsMap = $derived(new Map(models.map((m) => [m.id, m])));
	let skillsMap = $derived(new Map(skills.map((s) => [s.id, s])));

	// Model IDs may be a real model, obot://<alias>, or a wildcard prefix, so
	// resolve a label rather than assuming a lookup hit.
	function modelLabel(id: string) {
		if (id.startsWith('obot://')) return `${id.slice('obot://'.length)} (default alias)`;
		if (id === '*') return 'All models';
		if (id.endsWith('*')) return `${id} (prefix match)`;
		const match = modelsMap.get(id);
		return match ? match.displayName || match.name || id : id;
	}

	function skillLabel(id: string) {
		const match = skillsMap.get(id);
		return match ? match.displayName || match.name || id : id;
	}

	// Admin-configured MCP servers live as catalog entries (npx/uvx/containerized/remote
	// templates) as well as multi-user servers, so both have to be listed here.
	let mcpServerOptions = $derived([
		...mcpEntries.map((entry) => ({
			id: entry.id,
			name: entry.manifest?.name || entry.id,
			detail: entry.manifest?.serverUserType === 'multiUser' ? 'Multi-user' : 'Single-user'
		})),
		...mcpCatalogServers.map((server) => ({
			id: server.id,
			name: server.manifest?.name || server.alias || server.id,
			detail: 'Server'
		}))
	]);

	let mcpQuery = $state('');
	let filteredMcpServerOptions = $derived(
		mcpQuery
			? mcpServerOptions.filter((o) => o.name.toLowerCase().includes(mcpQuery.toLowerCase()))
			: mcpServerOptions
	);
	let selectedMcpServers = $derived(agent.mcpServers ?? []);

	onMount(async () => {
		try {
			const [harnessList, providers, entries, servers, modelList, skillList, repos] =
				await Promise.all([
					AdminService.listHarnesses(),
					AdminService.listModelProviders(),
					AdminService.listMCPCatalogEntries(DEFAULT_MCP_CATALOG_ID, { all: true }),
					AdminService.listMCPCatalogServers(DEFAULT_MCP_CATALOG_ID, { all: true }),
					AdminService.listModels({ all: true }),
					AdminService.listAllSkills(),
					AdminService.listSkillRepositories()
				]);
			harnesses = harnessList;
			modelProviders = providers.filter((p) => p.configured);
			mcpEntries = entries;
			mcpCatalogServers = servers;
			models = modelList;
			skills = skillList;
			skillRepositories = repos;
		} catch (error) {
			errors.append(`Failed to load services: ${error}`);
		} finally {
			loadingServices = false;
		}
	});

	function toggleModelProvider(id: string) {
		const current = agent.modelProviders ?? [];
		agent.modelProviders = current.includes(id)
			? current.filter((p) => p !== id)
			: [...current, id];
	}

	function toggleMcpServer(id: string) {
		const current = agent.mcpServers ?? [];
		agent.mcpServers = current.includes(id) ? current.filter((s) => s !== id) : [...current, id];
	}

	function toManifest(a: typeof agent): HostedAgentManifest {
		return {
			name: a.name,
			description: a.description,
			icon: a.icon,
			iconDark: a.iconDark,
			harnessID: a.harnessID,
			gitRepo: a.gitRepo,
			modelProviders: a.modelProviders,
			models: a.models,
			mcpServers: a.mcpServers,
			skills: a.skills,
			env: a.env,
			questions: a.questions,
			allowUserMCPServers: a.allowUserMCPServers,
			allowUserSkills: a.allowUserSkills,
			allowUserModels: a.allowUserModels,
			allowUserGitRepo: a.allowUserGitRepo,
			maxInstancesPerUser: a.maxInstancesPerUser
		};
	}

	function validate(a: typeof agent) {
		if (!a.name || !a.harnessID) return false;
		if ((a.env ?? []).some((e) => !e.key)) return false;
		if ((a.maxInstancesPerUser ?? 0) < 0) return false;
		const questions = a.questions ?? [];
		if (questions.some((q) => !q.key)) return false;
		if (questions.some((q) => q.type === 'select' && (q.options ?? []).length === 0)) return false;
		const keys = questions.map((q) => q.key);
		if (new Set(keys).size !== keys.length) return false;
		return true;
	}

	async function revealSecrets(): Promise<Record<string, string>> {
		if (!agent.id) return {};
		return AdminService.revealHostedAgent(agent.id);
	}
</script>

<div
	class="flex h-full w-full flex-col gap-4"
	out:fly={{ x: 100, duration }}
	in:fly={{ x: 100, delay: duration }}
>
	<div class="flex grow flex-col gap-4" out:fly={{ x: -100, duration }} in:fly={{ x: -100 }}>
		{#if agent.id}
			<div class="flex w-full items-center justify-between gap-4">
				<h1 class="flex items-center gap-4 text-2xl font-semibold">{agent.name}</h1>
				{#if !readonly}
					<IconButton
						variant="danger2"
						tooltip={{ text: 'Delete Agent' }}
						onclick={() => (deleting = true)}
					>
						<Trash2 class="size-4" />
					</IconButton>
				{/if}
			</div>
		{/if}

		<div
			class="dark:bg-base-400 dark:border-base-400 bg-base-100 flex flex-col gap-6 rounded-lg border border-transparent p-4"
		>
			<div class="flex flex-col gap-2">
				<label for="hosted-agent-name" class="text-sm font-light">Name</label>
				<input
					id="hosted-agent-name"
					bind:value={agent.name}
					class="text-input-filled"
					disabled={readonly}
				/>
			</div>

			<div class="flex flex-col gap-2">
				<label for="hosted-agent-description" class="text-sm font-light">Description</label>
				<textarea
					id="hosted-agent-description"
					bind:value={agent.description}
					class="text-input-filled"
					rows="3"
					disabled={readonly}
				></textarea>
			</div>

			<div class="flex flex-col gap-2">
				<label for="hosted-agent-harness" class="text-sm font-light">Harness</label>
				<select
					id="hosted-agent-harness"
					bind:value={agent.harnessID}
					class="text-input-filled"
					disabled={readonly || loadingServices}
				>
					<option value="" disabled>Select a harness...</option>
					{#each harnesses as harness (harness.id)}
						<option value={harness.id}>{harness.name}</option>
					{/each}
				</select>
				{#if !loadingServices && harnesses.length === 0}
					<span class="text-muted-content text-xs">
						No harnesses configured. Add one in the Harnesses tab first.
					</span>
				{/if}
			</div>

			<div class="flex flex-col gap-2">
				<label for="hosted-agent-git-repo" class="text-sm font-light">Git Repository</label>
				<input
					id="hosted-agent-git-repo"
					bind:value={agent.gitRepo}
					class="text-input-filled"
					placeholder="https://github.com/example/repo (optional)"
					disabled={readonly}
					inputmode="url"
					autocomplete="off"
				/>
			</div>

			<div class="flex flex-col gap-2">
				<label for="hosted-agent-icon" class="text-sm font-light">Icon URL</label>
				<div class="flex items-center gap-3">
					{#if agent.icon}
						<img src={agent.icon} alt="" class="size-10 shrink-0 rounded-md object-contain" />
					{/if}
					<input
						type="text"
						id="hosted-agent-icon"
						name="icon"
						bind:value={agent.icon}
						class="text-input-filled grow"
						disabled={readonly}
						inputmode="url"
						autocomplete="off"
					/>
				</div>
			</div>

			<div class="flex flex-col gap-2">
				<label for="hosted-agent-icon-dark" class="text-sm font-light">Icon URL (Dark)</label>
				<div class="flex items-center gap-3">
					{#if agent.iconDark}
						<img
							src={agent.iconDark}
							alt=""
							class="bg-base-300 size-10 shrink-0 rounded-md object-contain"
						/>
					{/if}
					<input
						type="text"
						id="hosted-agent-icon-dark"
						name="iconDark"
						bind:value={agent.iconDark}
						class="text-input-filled grow"
						disabled={readonly}
						inputmode="url"
						autocomplete="off"
					/>
				</div>
			</div>
		</div>

		<div class="flex flex-col gap-2">
			<div class="mb-2 flex flex-col">
				<h2 class="text-lg font-semibold">Services</h2>
				<span class="text-muted-content text-xs">
					Model providers, models, MCP servers, and skills made available to the agent.
				</span>
			</div>
			{#if loadingServices}
				<div class="my-2 flex items-center justify-center">
					<Loading class="size-6" />
				</div>
			{:else}
				<div
					class="dark:bg-base-400 dark:border-base-400 bg-base-100 flex flex-col gap-6 rounded-lg border border-transparent p-4"
				>
					<div class="flex flex-col gap-2">
						<span class="text-sm font-light">Model Providers</span>
						{#if modelProviders.length === 0}
							<p class="text-muted-content text-sm">No configured model providers.</p>
						{:else}
							<div class="flex flex-col gap-1">
								{#each modelProviders as provider (provider.id)}
									<label class="flex items-center gap-2 text-sm font-light">
										<input
											type="checkbox"
											class="checkbox checkbox-sm"
											checked={agent.modelProviders?.includes(provider.id)}
											onchange={() => toggleModelProvider(provider.id)}
											disabled={readonly}
										/>
										{provider.name}
									</label>
								{/each}
							</div>
						{/if}
					</div>

					<div class="flex flex-col gap-2">
						<div class="flex items-center justify-between">
							<span class="text-sm font-light">Models</span>
							{#if !readonly}
								<button
									class="btn btn-secondary flex items-center gap-1 text-xs"
									onclick={() => addModelDialog?.open()}
								>
									<Plus class="size-3" /> Add Models
								</button>
							{/if}
						</div>
						{#if (agent.models ?? []).length === 0}
							<p class="text-muted-content text-sm">No models added.</p>
						{:else}
							<div class="flex flex-col gap-1">
								{#each agent.models ?? [] as id (id)}
									<div class="flex items-center justify-between gap-2 text-sm font-light">
										<span class="truncate">{modelLabel(id)}</span>
										{#if !readonly}
											<IconButton
												variant="danger"
												onclick={() => {
													agent.models = (agent.models ?? []).filter((m) => m !== id);
												}}
												tooltip={{ text: 'Remove Model' }}
											>
												<Trash2 class="size-4" />
											</IconButton>
										{/if}
									</div>
								{/each}
							</div>
						{/if}
					</div>

					<div class="flex flex-col gap-2">
						<div class="flex items-center justify-between">
							<span class="text-sm font-light">Skills</span>
							{#if !readonly}
								<button
									class="btn btn-secondary flex items-center gap-1 text-xs"
									onclick={() => addSkillDialog?.open()}
								>
									<Plus class="size-3" /> Add Skills
								</button>
							{/if}
						</div>
						{#if (agent.skills ?? []).length === 0}
							<p class="text-muted-content text-sm">No skills added.</p>
						{:else}
							<div class="flex flex-col gap-1">
								{#each agent.skills ?? [] as id (id)}
									<div class="flex items-center justify-between gap-2 text-sm font-light">
										<span class="truncate">{skillLabel(id)}</span>
										{#if !readonly}
											<IconButton
												variant="danger"
												onclick={() => {
													agent.skills = (agent.skills ?? []).filter((s) => s !== id);
												}}
												tooltip={{ text: 'Remove Skill' }}
											>
												<Trash2 class="size-4" />
											</IconButton>
										{/if}
									</div>
								{/each}
							</div>
						{/if}
					</div>

					<div class="flex flex-col gap-2">
						<div class="flex items-center justify-between">
							<span class="text-sm font-light">MCP Servers</span>
							{#if selectedMcpServers.length > 0}
								<span class="text-muted-content text-xs">{selectedMcpServers.length} selected</span>
							{/if}
						</div>

						{#if mcpServerOptions.length === 0}
							<p class="text-muted-content text-sm">No configured MCP servers.</p>
						{:else}
							<Search
								class="dark:bg-base-200 dark:border-base-400 shadow-inner dark:border"
								onChange={(val) => (mcpQuery = val)}
								value={mcpQuery}
								placeholder="Search MCP servers..."
							/>
							<div class="default-scrollbar-thin flex max-h-64 flex-col gap-1 overflow-y-auto">
								{#each filteredMcpServerOptions as option (option.id)}
									<label class="flex items-center gap-2 text-sm font-light">
										<input
											type="checkbox"
											class="checkbox checkbox-sm shrink-0"
											checked={agent.mcpServers?.includes(option.id)}
											onchange={() => toggleMcpServer(option.id)}
											disabled={readonly}
										/>
										<span class="truncate">{option.name}</span>
										<span class="badge badge-secondary badge-xs shrink-0">{option.detail}</span>
									</label>
								{/each}
								{#if filteredMcpServerOptions.length === 0}
									<p class="text-muted-content py-2 text-sm">No servers match "{mcpQuery}".</p>
								{/if}
							</div>
						{/if}
					</div>
				</div>
			{/if}
		</div>

		<HostedAgentEnvEditor
			bind:env={agent.env as NonNullable<typeof agent.env>}
			{readonly}
			onReveal={agent.id ? revealSecrets : undefined}
		/>

		<div class="flex flex-col gap-2">
			<div class="mb-2 flex flex-col">
				<h2 class="text-lg font-semibold">Instances</h2>
				<span class="text-muted-content text-xs">
					Each user creates their own instances of this agent.
				</span>
			</div>
			<div
				class="dark:bg-base-400 dark:border-base-400 bg-base-100 flex flex-col gap-4 rounded-lg border border-transparent p-4"
			>
				<div class="flex flex-col gap-2">
					<label for="hosted-agent-max-instances" class="text-sm font-light">
						Max instances per user
					</label>
					<input
						id="hosted-agent-max-instances"
						type="number"
						min="0"
						bind:value={agent.maxInstancesPerUser}
						class="text-input-filled w-40"
						disabled={readonly}
					/>
					<span class="text-muted-content text-xs">0 means unlimited.</span>
				</div>

				<div class="flex flex-col gap-2 pt-2">
					<span class="text-sm font-light">User-defined resources</span>
					<span class="text-muted-content text-xs">
						Let users attach their own resources to an instance, on top of the ones configured
						above. Users can only pick from what they already have access to.
					</span>
					<label class="flex items-center gap-2 pt-1 text-sm font-light">
						<input
							type="checkbox"
							class="checkbox checkbox-sm"
							bind:checked={agent.allowUserMCPServers}
							disabled={readonly}
						/>
						Allow user-defined MCP servers
					</label>
					<label class="flex items-center gap-2 text-sm font-light">
						<input
							type="checkbox"
							class="checkbox checkbox-sm"
							bind:checked={agent.allowUserSkills}
							disabled={readonly}
						/>
						Allow user-defined skills
					</label>
					<label class="flex items-center gap-2 text-sm font-light">
						<input
							type="checkbox"
							class="checkbox checkbox-sm"
							bind:checked={agent.allowUserModels}
							disabled={readonly}
						/>
						Allow user-defined models
					</label>
					<label class="flex items-center gap-2 text-sm font-light">
						<input
							type="checkbox"
							class="checkbox checkbox-sm"
							bind:checked={agent.allowUserGitRepo}
							disabled={readonly}
						/>
						Allow user-specified git repository
					</label>
				</div>
			</div>
		</div>

		<HostedAgentQuestionsEditor
			bind:questions={agent.questions as NonNullable<typeof agent.questions>}
			{readonly}
		/>
	</div>

	{#if !readonly}
		<div
			class="bg-base-200 text-muted-content dark:bg-base-100 sticky bottom-0 left-0 z-50 flex w-full justify-end gap-2 py-4"
		>
			<div class="flex w-full justify-end gap-2">
				{#if !agent.id}
					<button class="btn btn-secondary text-sm" onclick={() => goto('/admin/hosted-agents')}>
						Cancel
					</button>
					<button
						class="btn btn-primary text-sm"
						disabled={!validate(agent) || saving}
						onclick={async () => {
							saving = true;
							try {
								const response = await AdminService.createHostedAgent(toManifest(agent));
								agent = { ...emptyAgent(), ...response };
								onCreate?.(response);
							} finally {
								saving = false;
							}
						}}
					>
						{#if saving}
							<Loading class="size-4" />
						{:else}
							Save
						{/if}
					</button>
				{:else}
					<button
						class="btn btn-primary text-sm"
						disabled={!validate(agent) || saving}
						onclick={async () => {
							if (!agent.id) return;
							saving = true;
							try {
								const response = await AdminService.updateHostedAgent(agent.id, toManifest(agent));
								agent = { ...emptyAgent(), ...response };
								onUpdate?.(response);
							} finally {
								saving = false;
							}
						}}
					>
						{#if saving}
							<Loading class="size-4" />
						{:else}
							Update
						{/if}
					</button>
				{/if}
			</div>
		</div>
	{/if}
</div>

<SearchModels
	bind:this={addModelDialog}
	{models}
	defaultAliases={defaultModelAliases}
	exclude={agent.models ?? []}
	onAdd={(modelIds: string[]) => {
		const existing = new Set(agent.models ?? []);
		agent.models = [...(agent.models ?? []), ...modelIds.filter((id) => !existing.has(id))];
	}}
/>

<SearchSkills
	bind:this={addSkillDialog}
	{skills}
	{skillRepositories}
	wildcardAvailable={false}
	exclude={agent.skills ?? []}
	onAdd={(resources: SkillAccessPolicyResource[]) => {
		// The agent references individual skills, so ignore repository-level picks.
		const existing = new Set(agent.skills ?? []);
		const picked = resources
			.filter((r) => r.type === 'skill')
			.map((r) => r.id)
			.filter((id) => !existing.has(id));
		agent.skills = [...(agent.skills ?? []), ...picked];
	}}
/>

<Confirm
	msg={`Delete ${agent.name || 'this agent'}?`}
	show={deleting}
	onsuccess={async () => {
		if (!agent.id) return;
		saving = true;
		await AdminService.deleteHostedAgent(agent.id);
		goto('/admin/hosted-agents');
	}}
	oncancel={() => (deleting = false)}
/>

<script lang="ts">
	import InfoTooltip from '$lib/components/InfoTooltip.svelte';
	import SensitiveInput from '$lib/components/SensitiveInput.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		NanobotService,
		UserService,
		type HostedAgent,
		type HostedAgentQuestion,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type Model
	} from '$lib/services';
	import type { Skill } from '$lib/services/nanobot/types';
	import { errors } from '$lib/stores';
	import { onMount } from 'svelte';

	interface Props {
		agent: HostedAgent;
		name: string;
		description: string;
		icon: string;
		answers: Record<string, string>;
		gitRepo: string;
		mcpServers: string[];
		skills: string[];
		models: string[];
	}

	let {
		agent,
		name = $bindable(''),
		description = $bindable(''),
		icon = $bindable(''),
		answers = $bindable({}),
		gitRepo = $bindable(''),
		mcpServers = $bindable([]),
		skills = $bindable([]),
		models = $bindable([])
	}: Props = $props();

	let questions = $derived(agent.questions ?? []);
	let allowsAnyUserResource = $derived(
		Boolean(agent.allowUserMCPServers || agent.allowUserSkills || agent.allowUserModels)
	);

	let loading = $state(false);
	let userMcpOptions = $state<{ id: string; name: string }[]>([]);
	let userSkills = $state<Skill[]>([]);
	let userModels = $state<Model[]>([]);

	onMount(async () => {
		if (!allowsAnyUserResource) return;
		loading = true;
		try {
			// Deliberately the user-scoped endpoints: a user may only attach things
			// they already have access to.
			const [entries, servers, skillList, modelList] = await Promise.all([
				agent.allowUserMCPServers ? UserService.listMCPs() : Promise.resolve([]),
				agent.allowUserMCPServers ? UserService.listMCPCatalogServers() : Promise.resolve([]),
				agent.allowUserSkills
					? NanobotService.listSkills({ dontLogErrors: true })
					: Promise.resolve([]),
				agent.allowUserModels ? UserService.listModels() : Promise.resolve([])
			]);
			userMcpOptions = [
				...(entries as MCPCatalogEntry[]).map((e) => ({
					id: e.id,
					name: e.manifest?.name || e.id
				})),
				...(servers as MCPCatalogServer[]).map((s) => ({
					id: s.id,
					name: s.manifest?.name || s.alias || s.id
				}))
			];
			userSkills = skillList as Skill[];
			userModels = modelList as Model[];
		} catch (error) {
			errors.append(`Failed to load your available resources: ${error}`);
		} finally {
			loading = false;
		}
	});

	function toggle(list: string[], id: string) {
		return list.includes(id) ? list.filter((x) => x !== id) : [...list, id];
	}

	function questionLabel(q: HostedAgentQuestion) {
		return q.name || q.key;
	}

	function placeholderFor(q: HostedAgentQuestion) {
		if (q.type === 'schedule') return q.default || '0 3 * * *';
		return q.default ?? '';
	}
</script>

<div class="flex flex-col gap-4">
	<div class="flex flex-col gap-2">
		<label for="instance-name" class="text-sm font-light">Name</label>
		<input id="instance-name" bind:value={name} class="text-input-filled" />
	</div>
	<div class="flex flex-col gap-2">
		<label for="instance-description" class="text-sm font-light">Description</label>
		<textarea id="instance-description" bind:value={description} class="text-input-filled" rows="2"
		></textarea>
	</div>
	<div class="flex flex-col gap-2">
		<label for="instance-icon" class="text-sm font-light">Icon URL</label>
		<div class="flex items-center gap-3">
			{#if icon}
				<img src={icon} alt="" class="size-10 shrink-0 rounded-md object-contain" />
			{/if}
			<input
				type="text"
				id="instance-icon"
				name="icon"
				bind:value={icon}
				class="text-input-filled grow"
				inputmode="url"
				autocomplete="off"
			/>
		</div>
	</div>

	{#if agent.allowUserGitRepo}
		<div class="flex flex-col gap-2">
			<label for="instance-git-repo" class="text-sm font-light">Git Repository</label>
			<input
				id="instance-git-repo"
				bind:value={gitRepo}
				class="text-input-filled"
				placeholder={agent.gitRepo || 'https://github.com/example/repo (optional)'}
				inputmode="url"
				autocomplete="off"
			/>
			{#if agent.gitRepo}
				<span class="text-muted-content text-xs">
					Leave blank to use the agent's default repository.
				</span>
			{/if}
		</div>
	{/if}

	{#if questions.length > 0}
		<div class="border-surface3 flex flex-col gap-4 border-t pt-4">
			{#each questions as question (question.key)}
				<div class="flex flex-col gap-2">
					<div class="flex items-center gap-1">
						<label for="q-{question.key}" class="text-sm font-light">
							{questionLabel(question)}
							{#if question.required}
								<span class="text-red-500">*</span>
							{/if}
						</label>
						{#if question.description}
							<InfoTooltip text={question.description} />
						{/if}
					</div>

					{#if question.type === 'select'}
						<select
							id="q-{question.key}"
							class="text-input-filled"
							bind:value={answers[question.key]}
						>
							{#if !question.required}
								<option value="">(none)</option>
							{/if}
							{#each question.options ?? [] as option (option)}
								<option value={option}>{option}</option>
							{/each}
						</select>
					{:else if question.type === 'boolean'}
						<select
							id="q-{question.key}"
							class="text-input-filled"
							bind:value={answers[question.key]}
						>
							<option value="true">Yes</option>
							<option value="false">No</option>
						</select>
					{:else if question.sensitive}
						<SensitiveInput bind:value={answers[question.key]} name="q-{question.key}" />
					{:else}
						<input
							id="q-{question.key}"
							type={question.type === 'number' ? 'number' : 'text'}
							bind:value={answers[question.key]}
							class="text-input-filled"
							placeholder={placeholderFor(question)}
						/>
					{/if}

					{#if question.type === 'schedule'}
						<span class="text-muted-content text-xs">
							Cron expression — minute hour day month weekday. For example
							<code>0 3 * * *</code> runs daily at 03:00.
						</span>
					{/if}
				</div>
			{/each}
		</div>
	{/if}

	{#if allowsAnyUserResource}
		<div class="border-surface3 flex flex-col gap-4 border-t pt-4">
			{#if loading}
				<div class="flex items-center justify-center py-2">
					<Loading class="size-5" />
				</div>
			{:else}
				{#if agent.allowUserMCPServers}
					<div class="flex flex-col gap-2">
						<span class="text-sm font-light">Your MCP servers</span>
						{#if userMcpOptions.length === 0}
							<p class="text-muted-content text-xs">You don't have access to any MCP servers.</p>
						{:else}
							<div class="default-scrollbar-thin flex max-h-40 flex-col gap-1 overflow-y-auto">
								{#each userMcpOptions as option (option.id)}
									<label class="flex items-center gap-2 text-sm font-light">
										<input
											type="checkbox"
											class="checkbox checkbox-sm shrink-0"
											checked={mcpServers.includes(option.id)}
											onchange={() => (mcpServers = toggle(mcpServers, option.id))}
										/>
										<span class="truncate">{option.name}</span>
									</label>
								{/each}
							</div>
						{/if}
					</div>
				{/if}

				{#if agent.allowUserSkills}
					<div class="flex flex-col gap-2">
						<span class="text-sm font-light">Your skills</span>
						{#if userSkills.length === 0}
							<p class="text-muted-content text-xs">You don't have access to any skills.</p>
						{:else}
							<div class="default-scrollbar-thin flex max-h-40 flex-col gap-1 overflow-y-auto">
								{#each userSkills as skill (skill.id)}
									<label class="flex items-center gap-2 text-sm font-light">
										<input
											type="checkbox"
											class="checkbox checkbox-sm shrink-0"
											checked={skills.includes(skill.id)}
											onchange={() => (skills = toggle(skills, skill.id))}
										/>
										<span class="truncate">{skill.displayName || skill.name || skill.id}</span>
									</label>
								{/each}
							</div>
						{/if}
					</div>
				{/if}

				{#if agent.allowUserModels}
					<div class="flex flex-col gap-2">
						<span class="text-sm font-light">Your models</span>
						{#if userModels.length === 0}
							<p class="text-muted-content text-xs">You don't have access to any models.</p>
						{:else}
							<div class="default-scrollbar-thin flex max-h-40 flex-col gap-1 overflow-y-auto">
								{#each userModels as model (model.id)}
									<label class="flex items-center gap-2 text-sm font-light">
										<input
											type="checkbox"
											class="checkbox checkbox-sm shrink-0"
											checked={models.includes(model.id)}
											onchange={() => (models = toggle(models, model.id))}
										/>
										<span class="truncate">{model.displayName || model.name || model.id}</span>
									</label>
								{/each}
							</div>
						{/if}
					</div>
				{/if}
			{/if}
		</div>
	{/if}
</div>

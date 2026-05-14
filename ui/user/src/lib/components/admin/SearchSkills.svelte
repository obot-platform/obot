<script lang="ts">
	import type { SkillRepository, SkillAccessPolicyResource } from '$lib/services/admin/types';
	import type { Skill } from '$lib/services/nanobot/types';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import Search from '../Search.svelte';
	import { Check, PencilRuler } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		skills: Skill[];
		skillRepositories: SkillRepository[];
		onAdd: (resources: SkillAccessPolicyResource[]) => void;
		exclude?: string[];
		title?: string;
		wildcardAvailable?: boolean;
	}

	let {
		skills,
		skillRepositories,
		onAdd,
		exclude = [],
		title = 'Add Skills',
		wildcardAvailable = true
	}: Props = $props();
	let addSkillDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let query = $state('');
	let selected = $state<SkillAccessPolicyResource[]>([]);
	let selectedSet = $derived(new Set(selected.map((item) => item.id)));

	const sortedRepositoriesAndSkills = $derived.by(() => {
		// eslint-disable-next-line svelte/prefer-svelte-reactivity
		const skillsByRepository = new Map<string, Skill[]>();
		for (const skill of skills) {
			if (!skill.repoID) {
				continue;
			}
			if (!skillsByRepository.has(skill.repoID)) {
				skillsByRepository.set(skill.repoID, []);
			}
			skillsByRepository.get(skill.repoID)?.push(skill);
		}
		let items = [];
		for (const repository of skillRepositories) {
			if (!exclude.includes(repository.id)) {
				items.push({
					type: 'skillRepository',
					id: repository.id,
					name: repository.displayName,
					description: ''
				});
			}
			const relatedSkills = skillsByRepository.get(repository.id);
			if (relatedSkills) {
				items.push(
					...relatedSkills
						.filter((s) => !exclude.includes(s.id))
						.map((s) => ({
							type: 'skill',
							id: s.id,
							name: s.name,
							description: s.description || ''
						}))
				);
			}
		}
		return query
			? items.filter(
					(item) =>
						item.name?.toLowerCase().includes(query.toLowerCase()) ||
						item.description?.toLowerCase().includes(query.toLowerCase())
				)
			: items;
	});

	function toggleSelection(item: SkillAccessPolicyResource) {
		if (selectedSet.has(item.id)) {
			selected = selected.filter((existing) => existing.id !== item.id);
		} else {
			selected = [...selected, item];
		}
	}

	function handleAdd() {
		onAdd(selected);
		addSkillDialog?.close();
	}

	export function open() {
		selected = [];
		query = '';
		addSkillDialog?.open();
	}

	export function close() {
		addSkillDialog?.close();
	}
</script>

<ResponsiveDialog
	bind:this={addSkillDialog}
	{title}
	class="h-full w-full overflow-visible md:h-[500px] md:max-w-md"
	classes={{ header: 'p-4 md:pb-0', content: 'min-h-inherit p-0' }}
>
	<div class="default-scrollbar-thin flex grow flex-col gap-4 overflow-y-auto pt-1">
		<div class="flex flex-col gap-2">
			<div class="px-4">
				<Search
					class="dark:bg-surface1 dark:border-surface3 shadow-inner dark:border"
					onChange={(val) => (query = val)}
					value={query}
					placeholder="Search repositories & skills..."
				/>
			</div>

			<div class="flex flex-col">
				{#if wildcardAvailable && !exclude?.includes('*')}
					<button
						class={twMerge(
							'hover:bg-surface3 dark:hover:bg-surface1 flex items-center justify-between gap-4 px-4 py-3 text-left',
							selectedSet.has('*') && 'dark:bg-gray-920 bg-gray-50'
						)}
						onclick={() => toggleSelection({ type: 'selector', id: '*' })}
					>
						<div class="flex items-center gap-2">
							<div class="flex flex-col">
								<p class="font-medium">All Skills</p>
								<span class="text-on-surface1 text-xs">
									Grants access to all current and future skills
								</span>
							</div>
						</div>
						<div class="flex size-6 items-center justify-center">
							{#if selectedSet.has('*')}
								<Check class="text-primary size-6" />
							{/if}
						</div>
					</button>
				{/if}

				{#each sortedRepositoriesAndSkills as item (item.id)}
					<button
						class={twMerge(
							'hover:bg-surface3 dark:hover:bg-surface1 flex items-center justify-between gap-4 px-4 py-3 text-left',
							selectedSet.has(item.id) && 'dark:bg-gray-920 bg-gray-50'
						)}
						onclick={() => {
							if (item.id === '*') {
								toggleSelection({ type: 'selector', id: '*' });
							} else {
								toggleSelection({
									type: item.type as 'skillRepository' | 'skill',
									id: item.id
								});
							}
						}}
					>
						<div class="flex items-center gap-2">
							<div class="flex flex-col">
								<p class="font-medium">{item.name}</p>
								<span class="text-on-surface1 line-clamp-1 text-xs">
									{#if item.type === 'skillRepository'}
										Grants access to all skills in this repository
									{:else}
										{item.description}
									{/if}
								</span>
							</div>
						</div>
						<div class="flex size-6 items-center justify-center">
							{#if selectedSet.has(item.id)}
								<Check class="text-primary size-6" />
							{/if}
						</div>
					</button>
				{/each}
			</div>
		</div>
	</div>
	<div class="flex w-full flex-col justify-between gap-4 p-4 md:flex-row">
		<div class="flex items-center gap-1 font-light">
			{#if selected.length > 0}
				<PencilRuler class="size-4" />
				{selected.length} Selected
			{/if}
		</div>
		<div class="flex items-center gap-2">
			<button class="button w-full md:w-fit" onclick={() => addSkillDialog?.close()}>
				Cancel
			</button>
			<button class="button-primary w-full md:w-fit" onclick={handleAdd}> Confirm </button>
		</div>
	</div>
</ResponsiveDialog>

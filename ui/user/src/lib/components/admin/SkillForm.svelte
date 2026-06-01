<script lang="ts">
	import { autoHeight } from '$lib/actions/textarea';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import Loading from '$lib/icons/Loading.svelte';
	import { AdminService } from '$lib/services';
	import type { Skill } from '$lib/services/nanobot/types';
	import MarkdownEditor from '../nanobot/MarkdownEditor.svelte';
	import { ExternalLink, Info } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';

	interface Props {
		skill: Skill;
	}

	let { skill }: Props = $props();
	let skillPreviewLoading = $state(false);
	let skillPreviewContent = $state<string>('');
	let activeTab = $state<'details' | 'preview'>('preview');

	const duration = PAGE_TRANSITION_DURATION;

	onMount(() => {
		if (skill) {
			skillPreviewLoading = true;
			AdminService.getSkillPreview(skill.id)
				.then((preview) => {
					skillPreviewContent = preview;
				})
				.catch((err) => {
					console.error(err);
					skillPreviewContent = 'Error loading skill preview';
				})
				.finally(() => {
					skillPreviewLoading = false;
				});
		}
	});
</script>

<div
	class="flex h-full w-full flex-col gap-4"
	out:fly={{ x: 100, duration }}
	in:fly={{ x: 100, delay: duration }}
>
	<div class="flex grow flex-col gap-4 pb-4" out:fly={{ x: -100, duration }} in:fly={{ x: -100 }}>
		<div class="flex w-full items-center justify-between gap-4">
			<h1 class="flex items-center gap-4 text-2xl font-semibold">
				{skill.displayName || 'Skill'}
			</h1>
			{#if skill.id}
				<a
					class="btn btn-primary"
					href={`${skill.repoURL}/tree/main/${skill.relativePath}`}
					rel="external noopener noreferrer"
					target="_blank"
				>
					View Source on Git<ExternalLink class="size-4" />
				</a>
			{/if}
		</div>

		{#if skill.repoURL}
			<div class="notification-info p-3 text-sm font-light">
				<div class="flex items-center gap-3">
					<Info class="size-6" />
					<div>
						<p>This skill comes from an external Git Source URL and cannot be edited.</p>
					</div>
				</div>
			</div>
		{/if}

		<div class="paper">
			<div class="flex flex-col gap-6">
				<div class="flex flex-col gap-2">
					<label for="skill-name" class="flex-1 text-sm font-light capitalize"> Name </label>
					<input
						id="skill-name"
						value={skill.displayName}
						class="text-input-filled mt-0.5"
						disabled
					/>
				</div>

				<div class="flex flex-col gap-2">
					<label for="skill-description" class="flex-1 text-sm font-light capitalize">
						Description
					</label>
					<textarea
						id="skill-description"
						value={skill.description}
						class="text-input-filled mt-0.5"
						disabled
						use:autoHeight
					></textarea>
				</div>
			</div>
		</div>

		<div class="paper p-0">
			<div class="border-base-300 flex gap-2 border-b">
				<button
					class="tab-button w-24 justify-center"
					class:tab-active={activeTab === 'preview'}
					onclick={() => (activeTab = 'preview')}>SKILL.MD</button
				>
				<button
					class="tab-button w-24 justify-center"
					class:tab-active={activeTab === 'details'}
					onclick={() => (activeTab = 'details')}>Details</button
				>
			</div>

			<div class="p-6 pt-2">
				{#if activeTab === 'details'}
					<div class="flex flex-col gap-6">
						<div class="flex flex-col gap-2">
							<label for="skill-repo-url" class="flex-1 text-sm font-light capitalize">
								Repository URL
							</label>
							<input
								id="skill-repo-url"
								value={skill.repoURL}
								class="text-input-filled mt-0.5"
								disabled
							/>
						</div>

						<div class="flex flex-col gap-2">
							<label for="skill-repo-ref" class="flex-1 text-sm font-light capitalize">
								Repository Reference
							</label>
							<input
								id="skill-repo-ref"
								value={skill.repoRef}
								class="text-input-filled mt-0.5"
								disabled
							/>
						</div>

						<div class="flex flex-col gap-2">
							<label for="skill-commit-sha" class="flex-1 text-sm font-light capitalize">
								Commit SHA
							</label>
							<input
								id="skill-commit-sha"
								value={skill.commitSHA}
								class="text-input-filled mt-0.5"
								disabled
							/>
						</div>
					</div>
					<div class="divider"></div>
					{#if skill.allowedTools || skill.compatibility || skill.license}
						<div class="flex flex-col gap-6">
							{#if skill.allowedTools}
								<div class="flex flex-col gap-2">
									<label for="skill-allowed-tools" class="flex-1 text-sm font-light capitalize">
										Allowed Tools
									</label>
									<input
										id="skill-allowed-tools"
										value={skill.allowedTools}
										class="text-input-filled mt-0.5"
										disabled
									/>
								</div>
							{/if}

							{#if skill.compatibility}
								<div class="flex flex-col gap-2">
									<label for="skill-compatibility" class="flex-1 text-sm font-light capitalize">
										Compatibility
									</label>
									<input
										id="skill-compatibility"
										value={skill.compatibility}
										class="text-input-filled mt-0.5"
										disabled
									/>
								</div>
							{/if}

							{#if skill.license}
								<div class="flex flex-col gap-2">
									<label for="skill-license" class="flex-1 text-sm font-light capitalize">
										License
									</label>
									<input
										id="skill-license"
										value={skill.license}
										class="text-input-filled mt-0.5"
										disabled
									/>
								</div>
							{/if}
						</div>
					{/if}
				{:else if skillPreviewLoading}
					<Loading />
				{:else if skillPreviewContent}
					<div class="nanobot skill-form-preview">
						<MarkdownEditor value={skillPreviewContent} readonly />
					</div>
				{/if}
			</div>
		</div>
	</div>
</div>

<style>
	:global(.skill-form-preview .milkdown) {
		--crepe-color-background: var(--color-base-100);

		:global(.dark) & {
			--crepe-color-background: var(--color-base-200);
		}
	}
</style>

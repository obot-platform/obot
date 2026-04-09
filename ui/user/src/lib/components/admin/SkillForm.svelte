<script lang="ts">
	import { autoHeight } from '$lib/actions/textarea';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import type { Skill } from '$lib/services/nanobot/types';
	import { Info } from 'lucide-svelte';
	import { fly } from 'svelte/transition';

	interface Props {
		skill: Skill;
	}

	let { skill }: Props = $props();
	const duration = PAGE_TRANSITION_DURATION;
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

		<div class="paper">
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
		</div>

		{#if skill.allowedTools || skill.compatibility || skill.license}
			<div class="paper">
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
			</div>
		{/if}
	</div>
</div>

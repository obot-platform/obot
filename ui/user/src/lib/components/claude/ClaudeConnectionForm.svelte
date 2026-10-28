<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { m } from '$lib/i18n';
	import {
		Group,
		UserService,
		type ClaudeConfig,
		type ClaudeEnvironment,
		type EnvironmentProfile,
		type VMCP
	} from '$lib/services';
	import { profile } from '$lib/stores';
	import IconButton from '../primitives/IconButton.svelte';
	import ActorResources from './ActorResources.svelte';
	import ClaudeConnectionProfileForm from './ClaudeConnectionProfileForm.svelte';
	import ClaudeConnectionProfiles from './ClaudeConnectionProfiles.svelte';
	import ClaudeTagSettings from './ClaudeTagSettings.svelte';
	import EgressHosts from './EgressHosts.svelte';
	import EgressSecrets from './EgressSecrets.svelte';
	import { Plus, Trash2 } from '@lucide/svelte';
	import { onMount, tick } from 'svelte';
	import { fly } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	type Draft = ClaudeEnvironment & { environmentKey: string };

	let {
		environment,
		onCancel,
		onSubmit
	}: {
		environment: string;
		onCancel: () => void;
		onSubmit: () => void;
	} = $props();

	let config = $state<ClaudeConfig>({ enabled: false });
	let servers = $state<VMCP[]>([]);
	let draft = $state<Draft>();
	let managedSettings = $state('');
	let error = $state('');
	let busy = $state(false);
	let ready = $state(false);
	let viewType = $state<'details' | 'profiles' | 'tag'>('details');
	let profileDraft = $state<EnvironmentProfile>();
	let editingProfileName = $state<string>();
	let profileForm = $state<ReturnType<typeof ClaudeConnectionProfileForm>>();
	let pendingView = $state<'details' | 'profiles' | 'tag'>();
	let confirmUnsaved = $state(false);
	let confirmDelete = $state(false);
	let confirmLLMProxy = $state(false);
	let applyingProfile = $state(false);
	const admin = $derived(profile.current.isAdmin?.() ?? false);

	function blankDraft(): Draft {
		return {
			id: '',
			displayName: '',
			organizationID: '',
			environmentID: '',
			environmentKey: '',
			claudeTagEnabled: false,
			profiles: [
				{
					name: 'default',
					subjects: [{ type: 'obotGroup', id: Group.ADMIN }],
					mcpServerIDs: null,
					resources: {}
				}
			],
			runtime: {
				// Blank images follow Obot's defaults; only images an admin enters are saved.
				image: '',
				workerImage: '',
				allowAllEgress: false,
				llmProxy: false,
				egressHosts: [],
				worker: {
					workerReplicas: 1,
					cpuLimit: '2',
					memoryLimit: '4Gi'
				},
				resources: {
					actorCPU: '1',
					actorMemory: '2Gi',
					idleTimeoutMinutes: 5,
					sessionRetentionDays: 30
				}
			}
		};
	}

	function applyEnvironment(selected: ClaudeEnvironment) {
		const copy = structuredClone(selected);
		managedSettings = copy.runtime.managedSettings
			? JSON.stringify(copy.runtime.managedSettings, null, 2)
			: '';
		draft = { ...copy, environmentKey: '' };
	}

	onMount(() => {
		const id = environment;
		const isAdmin = admin;
		let active = true;

		void (async () => {
			error = '';
			ready = false;
			try {
				const nextConfig = await UserService.getClaudeConfig();
				const nextServers = (await UserService.listVMCPs(isAdmin ? { all: true } : {})).filter(
					(server) => !isAdmin || !server.userID
				);
				const match =
					id === 'new' ? undefined : nextConfig.environments?.find((item) => item.id === id);
				const selected = match ? structuredClone(match) : undefined;
				if (!active) return;
				config = nextConfig;
				servers = nextServers;
				if (!nextConfig.enabled) {
					ready = true;
					return;
				}
				if (id === 'new') {
					draft = blankDraft();
					managedSettings = '';
				} else if (!selected) {
					error = m.hosted_agents_env_not_found();
				} else {
					applyEnvironment(selected);
				}
				ready = true;
			} catch (e) {
				if (!active) return;
				error = e instanceof Error ? e.message : String(e);
			}
		})();

		return () => {
			active = false;
		};
	});

	async function save(event: SubmitEvent) {
		event.preventDefault();
		if (!draft) return;
		busy = true;
		error = '';
		draft.runtime.egressHosts = draft.runtime.egressHosts
			.map((host) => host.trim())
			.filter(Boolean);
		for (const accessProfile of draft.profiles) {
			accessProfile.egressHosts = accessProfile.egressHosts
				?.map((host) => host.trim())
				.filter(Boolean);
		}
		try {
			const settings = managedSettings.trim() ? JSON.parse(managedSettings) : undefined;
			if (
				settings !== undefined &&
				(settings === null || typeof settings !== 'object' || Array.isArray(settings))
			) {
				throw new Error(m.hosted_agents_managed_settings_object());
			}
			draft.runtime.managedSettings = settings;
			delete (draft.runtime.worker as { nodeSelector?: unknown }).nodeSelector;

			const payload = { ...draft, organizationID: undefined, environmentID: undefined };
			if (draft.id) await UserService.updateClaudeEnvironment(draft.id, payload);
			else await UserService.createClaudeEnvironment(payload);
			onSubmit();
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			busy = false;
		}
	}

	async function refreshTag() {
		if (!draft?.id) return;
		const nextConfig = await UserService.getClaudeConfig();
		const match = nextConfig.environments?.find((item) => item.id === draft?.id);
		if (!match || !draft) return;
		draft.claudeTagEnabled = match.claudeTagEnabled;
		draft.claudeTagUserID = match.claudeTagUserID;
	}

	function openCreateProfile() {
		editingProfileName = undefined;
		profileDraft = { name: '', subjects: [], mcpServerIDs: null, resources: {} };
	}

	function openEditProfile(profile: EnvironmentProfile) {
		editingProfileName = profile.name;
		profileDraft = structuredClone($state.snapshot(profile));
	}

	function closeProfileForm() {
		profileDraft = undefined;
		editingProfileName = undefined;
	}

	function selectView(next: 'details' | 'profiles' | 'tag') {
		if (next === viewType) return;
		if (viewType === 'profiles' && profileDraft && profileForm?.hasUnsavedChanges()) {
			pendingView = next;
			confirmUnsaved = true;
			return;
		}
		viewType = next;
	}

	async function applyUnsavedAndSwitch() {
		if (applyingProfile) return;
		const formApi = profileForm;
		if (!formApi || !pendingView) return;
		if (!formApi.checkValidity()) {
			confirmUnsaved = false;
			pendingView = undefined;
			await tick();
			formApi.reportValidity();
			return;
		}
		applyingProfile = true;
		try {
			await formApi.applyUnsaved();
		} finally {
			applyingProfile = false;
		}
		if (pendingView) {
			confirmUnsaved = false;
			pendingView = undefined;
		}
	}

	function cancelUnsavedSwitch() {
		if (applyingProfile) return;
		confirmUnsaved = false;
		pendingView = undefined;
	}

	function saveProfiles(next: EnvironmentProfile[]) {
		if (!draft) return;
		draft.profiles = next;
		const nextView = pendingView;
		pendingView = undefined;
		confirmUnsaved = false;
		closeProfileForm();
		if (nextView) viewType = nextView;
	}

	async function remove() {
		if (!draft?.id || busy) return;
		busy = true;
		error = '';
		try {
			await UserService.deleteClaudeEnvironment(draft.id);
			confirmDelete = false;
			onSubmit();
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
			confirmDelete = false;
		} finally {
			busy = false;
		}
	}
</script>

{#if error}<p role="alert" class="text-error">{error}</p>{/if}
{#if ready && !config.enabled}
	<p class="text-muted-content">{m.hosted_agents_env_disabled()}</p>
{:else if draft}
	<div in:fly|global={{ x: 100, delay: PAGE_TRANSITION_DURATION }}>
		<div class="flex justify-between gap-4 items-center mb-3">
			<div class="w-fit">
				<div class="tabs tabs-box bg-base-100 shadow-sm dark:bg-base-300">
					<button
						class={twMerge(
							'tab text-sm min-w-24',
							viewType === 'details' && 'tab-active bg-base-300 dark:bg-base-100'
						)}
						onclick={() => selectView('details')}
					>
						{m.hosted_agents_details_tab()}
					</button>
					<button
						class={twMerge(
							'tab text-sm min-w-24',
							viewType === 'profiles' && 'tab-active bg-base-300 dark:bg-base-100'
						)}
						onclick={() => selectView('profiles')}
					>
						{m.hosted_agents_profiles_tab()}
					</button>

					{#if draft.id}
						<button
							class={twMerge(
								'tab text-sm min-w-24',
								viewType === 'tag' && 'tab-active bg-base-300 dark:bg-base-100'
							)}
							onclick={() => selectView('tag')}
						>
							{m.hosted_agents_tag_heading()}
						</button>
					{/if}
				</div>
			</div>

			{#if viewType === 'profiles' && !profileDraft && draft.profiles.length > 0}
				<button class="btn btn-primary" type="button" onclick={openCreateProfile}>
					<Plus class="size-4" />
					{m.hosted_agents_profile_create()}
				</button>
			{:else if viewType !== 'profiles' && draft.id}
				<IconButton
					disabled={busy}
					onclick={() => (confirmDelete = true)}
					tooltip={{ text: m.hosted_agents_env_action_delete() }}
					variant="danger2"
				>
					<Trash2 class="size-4" />
				</IconButton>
			{/if}
		</div>
		{#if viewType === 'profiles' && profileDraft}
			<ClaudeConnectionProfileForm
				bind:this={profileForm}
				environment={draft}
				profiles={draft.profiles}
				profile={profileDraft}
				editingName={editingProfileName}
				{servers}
				onClose={closeProfileForm}
				onSaved={saveProfiles}
			/>
		{:else if viewType === 'profiles'}
			<ClaudeConnectionProfiles
				environment={draft}
				onCreate={openCreateProfile}
				onEdit={openEditProfile}
				onUpdated={(next) => {
					if (draft) {
						draft.profiles = next;
					}
				}}
			/>
		{:else if viewType === 'tag' && draft.id}
			<ClaudeTagSettings environment={draft} onsaved={refreshTag} />
		{:else}
			<form id="claude-environment-form" class="space-y-5" onsubmit={save}>
				<fieldset disabled={busy} class="space-y-4">
					<div class="space-y-3 block paper">
						<label class="flex flex-col gap-1 text-sm">
							{m.hosted_agents_env_name()}<input
								class="text-input-filled"
								required
								maxlength="200"
								bind:value={draft.displayName}
							/>
						</label>
						<label class="flex flex-col gap-1 text-sm">
							{m.hosted_agents_env_key()}
							<input
								class="text-input-filled"
								type="password"
								autocomplete="new-password"
								required={!draft.id}
								bind:value={draft.environmentKey}
								placeholder={draft.id ? m.hosted_agents_env_keep_key() : ''}
							/>
						</label>
					</div>
					<fieldset class="space-y-3 block paper">
						<h3 class="font-semibold">{m.hosted_agents_env_pool()}</h3>

						<div class="grid gap-3 sm:grid-cols-2">
							<label class="flex flex-col gap-1 text-sm"
								>{m.hosted_agents_env_replicas()}<input
									class="text-input-filled"
									type="number"
									min="1"
									max="100"
									required
									bind:value={draft.runtime.worker.workerReplicas}
								/></label
							>
							<label class="flex flex-col gap-1 text-sm"
								>{m.hosted_agents_env_cpu()}<input
									class="text-input-filled"
									required
									bind:value={draft.runtime.worker.cpuLimit}
								/></label
							>
							<label class="flex flex-col gap-1 text-sm"
								>{m.hosted_agents_env_memory()}<input
									class="text-input-filled"
									required
									bind:value={draft.runtime.worker.memoryLimit}
								/></label
							>
							<label class="flex flex-col gap-1 text-sm"
								>{m.hosted_agents_env_worker_image()}<input
									class="text-input-filled"
									bind:value={draft.runtime.workerImage}
									placeholder={config.defaultWorkerImage}
								/></label
							>
						</div>
						<p class="text-xs text-muted-content">
							{m.hosted_agents_env_capacity_help()}
							{m.hosted_agents_env_image_default_help()}
						</p>
					</fieldset>
					<fieldset class="space-y-3 block paper">
						<h3 class="font-semibold">{m.hosted_agents_env_actor_defaults()}</h3>

						<ActorResources bind:resources={draft.runtime.resources} />
						<p class="text-xs text-muted-content">
							{m.hosted_agents_env_digest_help()}
							{m.hosted_agents_env_image_default_help()}
						</p>
						<details class="space-y-3">
							<summary class="cursor-pointer text-sm font-medium"
								>{m.hosted_agents_env_advanced()}</summary
							><label class="flex flex-col gap-1 text-sm"
								>{m.hosted_agents_env_runner_image()}<input
									class="text-input-filled"
									bind:value={draft.runtime.image}
									placeholder={config.defaultRunnerImage || m.hosted_agents_env_image_placeholder()}
									pattern={'[^@\\s]+@sha256:[a-f0-9]{64}'}
									maxlength={512}
									title={m.hosted_agents_env_digest_title()}
									oninvalid={(event) => {
										const details = event.currentTarget.closest('details');
										if (details) details.open = true;
									}}
								/></label
							>
							<label class="flex flex-col gap-1 text-sm">
								{m.hosted_agents_managed_settings_label()}
								<textarea
									class="text-input-filled font-mono"
									rows="10"
									bind:value={managedSettings}
									placeholder={'{}'}></textarea>
							</label>
							<p class="text-xs text-muted-content">{m.hosted_agents_managed_settings_help()}</p>

							<div class="divider"></div>

							<label class="flex items-center gap-2 text-sm">
								<input type="checkbox" bind:checked={draft.runtime.allowAllEgress} />
								{m.hosted_agents_allow_all_egress()}
							</label>
							<p class="text-xs text-muted-content">{m.hosted_agents_allow_all_egress_help()}</p>
							<p class="text-xs text-muted-content">{m.hosted_agents_required_egress_help()}</p>

							{#if !draft.runtime.allowAllEgress}
								<div class="divider"></div>
								<EgressHosts
									label={m.hosted_agents_environment_egress_hosts()}
									bind:hosts={draft.runtime.egressHosts}
									disabled={draft.runtime.allowAllEgress}
								/>
							{/if}
						</details>
					</fieldset>
					<fieldset class="space-y-3 block paper">
						<h3 class="font-semibold">{m.hosted_agents_llm_proxy_title()}</h3>
						<label class="flex items-center gap-2 text-sm">
							<input
								type="checkbox"
								checked={draft.runtime.llmProxy ?? false}
								onchange={(event) => {
									if (!draft) return;
									if (event.currentTarget.checked) {
										// Enable only once the admin acknowledges what it needs.
										event.currentTarget.checked = false;
										confirmLLMProxy = true;
									} else {
										draft.runtime.llmProxy = false;
									}
								}}
							/>
							{m.hosted_agents_llm_proxy_label()}
						</label>
						<p class="text-xs text-muted-content">{m.hosted_agents_llm_proxy_help()}</p>
					</fieldset>
					<EgressSecrets
						bind:secrets={
							() => draft?.egressSecrets ?? [],
							(secrets) => {
								if (draft) draft.egressSecrets = secrets;
							}
						}
					/>
				</fieldset>
			</form>
			<div
				class="sticky bottom-0 z-10 flex items-center gap-2 bg-base-100 p-4 dark:bg-base-100 justify-end"
			>
				<div class="flex justify-end gap-2">
					<button class="btn btn-ghost" type="button" disabled={busy} onclick={onCancel}>
						{m.common_cancel()}
					</button>
					<button
						class="btn btn-primary"
						type="submit"
						form="claude-environment-form"
						disabled={busy}
					>
						{m.core_save()}
					</button>
				</div>
			</div>
		{/if}
		<Confirm
			show={confirmUnsaved}
			type="info"
			title={m.hosted_agents_profile_unsaved_title()}
			msg={m.hosted_agents_profile_unsaved()}
			note={draft.id
				? m.hosted_agents_profile_unsaved_save()
				: m.hosted_agents_profile_unsaved_apply()}
			submitText={draft.id ? m.core_save() : m.common_apply()}
			cancelText={m.hosted_agents_profile_keep_editing()}
			loading={applyingProfile}
			onsuccess={() => {
				void applyUnsavedAndSwitch();
			}}
			oncancel={cancelUnsavedSwitch}
		/>
		<Confirm
			show={confirmLLMProxy}
			type="info"
			title={m.hosted_agents_llm_proxy_confirm_title()}
			submitText={m.hosted_agents_llm_proxy_confirm_submit()}
			onsuccess={() => {
				if (draft) draft.runtime.llmProxy = true;
				confirmLLMProxy = false;
			}}
			oncancel={() => (confirmLLMProxy = false)}
		>
			{#snippet msgContent()}
				<div class="flex flex-col gap-2 text-sm">
					<p>{m.hosted_agents_llm_proxy_confirm_msg()}</p>
					<ol class="list-decimal space-y-1 pl-5">
						<li>{m.hosted_agents_llm_proxy_requirement_subscriptions()}</li>
						<li>{m.hosted_agents_llm_proxy_requirement_profile()}</li>
					</ol>
				</div>
			{/snippet}
		</Confirm>
		<Confirm
			show={confirmDelete}
			title={m.hosted_agents_env_action_delete()}
			msg={m.hosted_agents_env_delete_question({ name: draft.displayName })}
			loading={busy}
			onsuccess={() => {
				void remove();
			}}
			oncancel={() => {
				if (!busy) confirmDelete = false;
			}}
		/>
	</div>
{/if}

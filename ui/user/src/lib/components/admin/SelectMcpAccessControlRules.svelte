<script lang="ts">
	import {
		AdminService,
		type AccessControlRule,
		type AccessControlRuleResource,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type OrgUser
	} from '$lib/services';
	import { twMerge } from 'tailwind-merge';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import InfoTooltip from '../InfoTooltip.svelte';
	import { Circle, CircleCheck, LoaderCircle } from 'lucide-svelte';
	import { goto } from '$app/navigation';
	import { ADMIN_SESSION_STORAGE } from '$lib/constants';

	interface Props {
		entry?: MCPCatalogEntry | MCPCatalogServer;
		onSubmit?: () => void;
	}

	let { entry, onSubmit }: Props = $props();

	let users = $state<OrgUser[]>([]);
	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let accessControlRules = $state<AccessControlRule[]>([]);
	let userMap = $derived(new Map(users.map((user) => [user.id, user])));

	let selectedRules = $state<string[]>([]);
	let savingRules = $state(false);

	export async function open() {
		accessControlRules = await AdminService.listAccessControlRules();
		users = await AdminService.listUsers();
		dialog?.open();
	}

	export async function close() {
		selectedRules = [];
		dialog?.close();
		onSubmit?.();
	}

	async function handleAddToRules() {
		if (!entry) return;
		savingRules = true;
		const mappedRules = new Map<string, AccessControlRule>(
			accessControlRules.map((rule) => [rule.id, rule])
		);
		const type = 'manifest' in entry ? 'mcpServer' : 'mcpServerCatalogEntry';
		for (const rule of selectedRules) {
			const mappedRule = mappedRules.get(rule);
			if (!mappedRule) continue;

			await AdminService.updateAccessControlRule(rule, {
				...mappedRule,
				resources: [
					...(mappedRule.resources ?? []),
					{ id: entry.id, type }
				] as AccessControlRuleResource[]
			});
		}

		savingRules = false;
		close();
	}

	function convertToUserDisplayName(id: string) {
		if (id === '*') return 'Everyone';
		const user = userMap.get(id);
		if (!user) return id;
		return user.email ?? user.username ?? id;
	}

	function handleCreateNewRule() {
		if (entry) {
			sessionStorage.setItem(ADMIN_SESSION_STORAGE.ACCESS_CONTROL_RULE_CREATION, entry.id);
		}
		goto('/v2/admin/access-control?new=true');
	}
</script>

<ResponsiveDialog
	bind:this={dialog}
	title="Add to Access Control Rule(s)"
	class="overflow-visible md:w-2xl"
>
	{#if accessControlRules.length === 0}
		<p class="text-md mb-4 font-light">Looks like you don't have any access control rules yet!</p>
		<p class="text-md mb-8 font-light">Want to go ahead & create one now?</p>
	{:else}
		<p class="text-md mb-8 font-light">
			Select the access control rules you want to apply to this MCP server.
		</p>
	{/if}
	{#if accessControlRules.length > 0}
		<div class="mb-8 flex flex-col">
			<div class="grid grid-cols-2 gap-2 pb-1 text-xs font-semibold uppercase">
				<p>Rule</p>
				<p>User/Groups</p>
			</div>
			<div class="flex flex-col gap-1">
				{#each accessControlRules as rule}
					{@const hasEverything = rule.resources?.find((r) => r.id === '*') !== undefined}
					<div class="flex items-center gap-2">
						<button
							class={twMerge(
								'flex w-full items-center gap-2 rounded-md border border-transparent p-2 text-left transition-colors duration-200',
								selectedRules.includes(rule.id) && 'border-blue-500',
								!hasEverything && 'dark:hover:bg-surface1 hover:bg-surface2'
							)}
							onclick={() => {
								if (hasEverything) return;
								if (selectedRules.includes(rule.id)) {
									selectedRules = selectedRules.filter((id) => id !== rule.id);
								} else {
									selectedRules.push(rule.id);
								}
							}}
						>
							<div class="grid w-full grid-cols-2 items-center gap-2">
								<p class={twMerge('truncate', hasEverything && 'text-gray-400 dark:text-gray-600')}>
									{rule.displayName}
								</p>
								<div class="flex grow items-center justify-between">
									<p
										class={twMerge(
											'line-clamp-2 text-xs',
											hasEverything && 'text-gray-400 dark:text-gray-600'
										)}
									>
										{#if rule.subjects && rule.subjects.length > 0}
											{rule.subjects?.map((s) => convertToUserDisplayName(s.id)).join(', ')}
										{:else}
											<i class="text-gray-400 dark:text-gray-600">(Empty)</i>
										{/if}
									</p>
									<div class="flex-shrink-0">
										{#if hasEverything}
											<InfoTooltip
												class="size-4"
												classes={{ icon: 'size-4' }}
												placement="top-end"
												text="This server will be available by default to everyone in this rule."
											/>
										{:else if selectedRules.includes(rule.id)}
											<CircleCheck class="size-4 text-blue-500" />
										{:else}
											<Circle class="size-4 text-gray-400 dark:text-gray-600" />
										{/if}
									</div>
								</div>
							</div>
						</button>
					</div>
				{/each}
			</div>
		</div>
	{/if}
	{#if accessControlRules.length > 0}
		<div class="mt-auto flex justify-between gap-4">
			<button class="button-primary" onclick={handleCreateNewRule}> Create New Rule </button>
			<div class="flex items-center gap-4">
				<button class="button" onclick={close}> Skip Step </button>
				<button
					class="button-primary flex items-center gap-1"
					onclick={handleAddToRules}
					disabled={savingRules}
				>
					{#if savingRules}
						<LoaderCircle class="size-4 animate-spin" />
					{:else}
						Continue
					{/if}
				</button>
			</div>
		</div>
	{:else}
		<div class="mt-auto flex justify-end gap-4">
			<button class="button" onclick={close}> Skip Step </button>
			<button class="button-primary" onclick={handleCreateNewRule}>
				Create Access Control Rule
			</button>
		</div>
	{/if}
</ResponsiveDialog>

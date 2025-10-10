<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { AdminService, type ProjectThread, type Project, type OrgUser } from '$lib/services';
	import { Eye, LoaderCircle, MessageCircle } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';
	import { replaceState } from '$app/navigation';
	import { formatTimeAgo } from '$lib/time';
	import Search from '$lib/components/Search.svelte';
	import { page } from '$app/state';
	import { debounce } from 'es-toolkit';
	import { Group } from '$lib/services/admin/types';
	import { openUrl } from '$lib/utils';
	import { profile } from '$lib/stores';
	import { twMerge } from 'tailwind-merge';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { clearUrlParams, setUrlParams } from '$lib/url';

	let threads = $state<ProjectThread[]>([]);
	let projects = $state<Project[]>([]);
	let users = $state<OrgUser[]>([]);
	let projectMap = $derived(new Map(projects.map((p) => [p.id, p.name])));
	let userMap = $derived(new Map(users.map((u) => [u.id, u])));

	let query = $state(page.url.searchParams.get('query') || '');
	let urlFilters = $state<Record<string, (string | number)[]>>({});

	let loading = $state(true);
	let filteredThreads = $derived(threads.filter((thread) => !thread.project && !thread.systemTask));
	let tableData = $derived(
		filteredThreads.map((thread) => {
			return {
				...thread,
				displayName: formatThreadName(thread),
				projectName: projectMap.get(thread.projectID || '') || thread.projectID,
				userName: userMap.get(thread.userID || '')?.displayName || '-',
				userEmail: userMap.get(thread.userID || '')?.email || '-'
			};
		})
	);

	const updateQuery = debounce((value: string) => {
		query = value;

		if (value) {
			page.url.searchParams.set('query', value);
		} else {
			page.url.searchParams.delete('query');
		}

		// Update the query search param without cause app to react
		// Prevent losing focus from the input
		// history.replaceState(null, '', page.url);
		replaceState(page.url, { query });
	}, 100);

	onMount(() => {
		loadThreads();
		if (page.url.searchParams.size > 0) {
			page.url.searchParams.forEach((value, key) => {
				urlFilters[key] = value.split(',');
			});
		}
	});

	async function loadThreads() {
		loading = true;
		try {
			// Load threads, projects, and users in parallel with individual error handling
			const threadsPromise = AdminService.listThreads().catch((err) => {
				console.error('Failed to load threads:', err);
				return [];
			});

			const projectsPromise = AdminService.listProjects().catch((err) => {
				console.error('Failed to load projects:', err);
				return [];
			});

			const usersPromise = AdminService.listUsers().catch((err) => {
				console.error('Failed to load users:', err);
				return [];
			});

			// Add timeout to prevent hanging
			const timeoutPromise = new Promise<never>((_, reject) => {
				setTimeout(() => reject(new Error('Request timeout')), 10000);
			});

			const [threadsData, projectsData, usersData] = await Promise.race([
				Promise.all([threadsPromise, projectsPromise, usersPromise]),
				timeoutPromise
			]);

			// threads = threadsData;
			projects = projectsData;
			users = usersData;
			// Filter out task & task runs
			threads = threadsData.filter((thread) => !thread.taskID && !thread.taskRunID);
		} catch (error) {
			console.error('Failed to load data:', error);
			// Set empty arrays as fallback
			threads = [];
			projects = [];
			users = [];
			// filteredThreads = [];
		} finally {
			loading = false;
		}
	}

	function formatThreadName(thread: ProjectThread) {
		return thread.name || 'Unnamed Thread';
	}
	let isAuditor = $derived(profile.current.groups.includes(Group.AUDITOR));
</script>

<Layout>
	<div
		class="my-4 h-full w-full"
		in:fly={{ x: 100, duration: 300, delay: 150 }}
		out:fly={{ x: -100, duration: 300 }}
	>
		<div class="flex flex-col gap-8 pb-8">
			<h1 class="text-2xl font-semibold">Chat Threads</h1>

			<div class="flex flex-col gap-2">
				<div class="flex items-center gap-4">
					<Search
						value={query}
						class="dark:bg-surface1 dark:border-surface3 border border-transparent bg-white shadow-sm"
						onChange={updateQuery}
						placeholder="Search threads..."
					/>
				</div>

				{#if loading}
					<div class="flex w-full justify-center py-12">
						<LoaderCircle class="size-8 animate-spin text-blue-600" />
					</div>
				{:else if filteredThreads.length === 0}
					<div class="flex w-full flex-col items-center justify-center py-12 text-center">
						<MessageCircle class="size-24 text-gray-200 dark:text-gray-700" />
						<h3 class="mt-4 text-lg font-semibold text-gray-400 dark:text-gray-600">
							No threads available
						</h3>
						<p class="mt-2 text-sm font-light text-gray-400 dark:text-gray-600">
							Threads will appear here once they are created.
						</p>
					</div>
				{:else}
					<Table
						data={tableData}
						fields={['displayName', 'userName', 'userEmail', 'projectName', 'created']}
						filterable={['displayName', 'userName', 'userEmail', 'projectName']}
						filters={urlFilters}
						onFilter={setUrlParams}
						onClearAllFilters={clearUrlParams}
						onClickRow={isAuditor
							? (d, isCtrlClick) => {
									const url = `/admin/chat-threads/${d.id}`;
									openUrl(url, isCtrlClick);
								}
							: undefined}
						headers={[
							{
								title: 'Name',
								property: 'displayName'
							},
							{
								title: 'User Name',
								property: 'userName'
							},
							{
								title: 'User Email',
								property: 'userEmail'
							},
							{
								title: 'Project',
								property: 'projectName'
							},
							{
								title: 'Created',
								property: 'created'
							}
						]}
						headerClasses={[
							{
								property: 'displayName',
								class: 'w-4/12 min-w-sm'
							}
						]}
						sortable={['displayName', 'userName', 'userEmail', 'projectName', 'created']}
						initSort={{ property: 'created', order: 'desc' }}
					>
						{#snippet actions()}
							<button
								class={twMerge(
									'icon-button',
									isAuditor && 'hover:text-blue-500',
									!isAuditor && 'opacity-50 hover:bg-transparent dark:hover:bg-transparent'
								)}
								title="View Thread"
								use:tooltip={{
									text: isAuditor
										? 'View Thread'
										: 'To view details, auditing permissions are required.'
								}}
							>
								<Eye class="size-4" />
							</button>
						{/snippet}
						{#snippet onRenderColumn(property, thread)}
							{#if property === 'created'}
								<span class="text-sm text-gray-600 dark:text-gray-400">
									{formatTimeAgo(thread.created).relativeTime}
								</span>
							{:else}
								{thread[property as keyof typeof thread]}
							{/if}
						{/snippet}
					</Table>
				{/if}
			</div>
		</div>
	</div>
</Layout>

<svelte:head>
	<title>Obot | Admin - Chat Threads</title>
</svelte:head>

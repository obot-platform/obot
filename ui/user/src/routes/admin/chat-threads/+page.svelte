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
	import {
		clearUrlParams,
		getTableUrlParamsFilters,
		getTableUrlParamsSort,
		setSearchParamsToLocalStorage,
		setSortUrlParams,
		setFilterUrlParams
	} from '$lib/url';

	let threads = $state<ProjectThread[]>([]);
	let projects = $state<Project[]>([]);
	let users = $state<OrgUser[]>([]);
	let projectMap = $derived(new Map(projects.map((p) => [p.id, p.name])));
	let userMap = $derived(new Map(users.map((u) => [u.id, u])));

	let query = $state(page.url.searchParams.get('query') || '');
	let urlFilters = $derived(getTableUrlParamsFilters());
	let initSort = $derived(getTableUrlParamsSort({ property: 'created', order: 'desc' }));

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

<Layout title="Chat Threads">
	<div
		class="h-full w-full"
		in:fly={{ x: 100, duration: 300, delay: 150 }}
		out:fly={{ x: -100, duration: 300 }}
	>
		<div class="flex flex-col gap-8 pb-8">
			<div class="flex flex-col gap-2">
				<div class="flex items-center gap-4">
					<Search
						value={query}
						class="dark:bg-surface1 dark:border-surface3 bg-background border border-transparent shadow-sm"
						onChange={updateQuery}
						placeholder="Search threads..."
					/>
				</div>

				{#if loading}
					<div class="flex w-full justify-center py-12">
						<LoaderCircle class="text-primary size-8 animate-spin" />
					</div>
				{:else if filteredThreads.length === 0}
					<div class="flex w-full flex-col items-center justify-center py-12 text-center">
						<MessageCircle class="text-on-surface1 size-24 opacity-50" />
						<h3 class="text-on-surface1 mt-4 text-lg font-semibold">No threads available</h3>
						<p class="text-on-surface1 mt-2 text-sm font-light">
							Threads will appear here once they are created.
						</p>
					</div>
				{:else}
					<Table
						data={tableData}
						fields={['displayName', 'userName', 'userEmail', 'projectName', 'created']}
						filterable={['displayName', 'userName', 'userEmail', 'projectName']}
						filters={urlFilters}
						onFilter={setFilterUrlParams}
						onClearAllFilters={clearUrlParams}
						onClickRow={isAuditor
							? (d, isCtrlClick) => {
									setSearchParamsToLocalStorage(page.url.pathname, page.url.search);
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
						{initSort}
						onSort={setSortUrlParams}
					>
						{#snippet actions()}
							<button
								class={twMerge(
									'icon-button',
									isAuditor && 'hover:text-primary',
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
								<span class="text-on-surface1 text-sm">
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

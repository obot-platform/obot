<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import {
		AdminService,
		type ProjectThread,
		type Project,
		type OrgUser,
		type Task,
		Group
	} from '$lib/services';
	import { Eye, LoaderCircle, MessageCircle } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';
	import { replaceState } from '$app/navigation';
	import { formatTimeAgo } from '$lib/time';
	import Search from '$lib/components/Search.svelte';
	import { page } from '$app/state';
	import { debounce } from 'es-toolkit';
	import { profile } from '$lib/stores';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { twMerge } from 'tailwind-merge';
	import { openUrl } from '$lib/utils';
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
	let tasks = $state<Task[]>([]);

	let projectMap = $derived(new Map(projects.map((p) => [p.id, p.name])));
	let userMap = $derived(new Map(users.map((u) => [u.id, u])));
	let taskMap = $derived(new Map(tasks.map((t) => [t.id, t])));

	let query = $state(page.url.searchParams.get('query') || '');
	let urlFilters = $derived(getTableUrlParamsFilters());
	let initSort = $derived(getTableUrlParamsSort());

	let loading = $state(true);
	let filteredThreads = $derived(threads.filter((thread) => !thread.project && !thread.systemTask));
	let tableData = $derived(
		filteredThreads.map((thread) => {
			return {
				...thread,
				projectName: projectMap.get(thread.projectID || '') || thread.projectID,
				userName: userMap.get(thread.userID || '')?.displayName || '-',
				userEmail: userMap.get(thread.userID || '')?.email || '-',
				task: taskMap.get(thread.taskID || '')?.name || '-'
			};
		})
	);

	let convertedUrlFilters = $derived.by(() => {
		const { task, ...rest } = urlFilters;
		// Convert task to taskID for filtering
		if (task) {
			return {
				...rest,
				taskID: task
			};
		}
		return rest;
	});

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

	let isAuditor = $derived(profile.current.groups.includes(Group.AUDITOR));

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

			const tasksPromise = AdminService.listTasks().catch((err) => {
				console.error('Failed to load tasks:', err);
				return [];
			});

			// Add timeout to prevent hanging
			const timeoutPromise = new Promise<never>((_, reject) => {
				setTimeout(() => reject(new Error('Request timeout')), 10000);
			});

			const [threadsData, projectsData, usersData, tasksData] = await Promise.race([
				Promise.all([threadsPromise, projectsPromise, usersPromise, tasksPromise]),
				timeoutPromise
			]);

			// threads = threadsData;
			projects = projectsData;
			users = usersData;
			// Filter to only include task runs
			threads = threadsData.filter((thread) => !!thread.taskRunID);
			tasks = tasksData;
		} catch (error) {
			console.error('Failed to load data:', error);
			// Set empty arrays as fallback
			threads = [];
			projects = [];
			users = [];
			tasks = [];
		} finally {
			loading = false;
		}
	}

	function handleViewThread(thread: ProjectThread, isCtrlClick: boolean) {
		setSearchParamsToLocalStorage(page.url.pathname, page.url.search);

		const url = `/admin/task-runs/${thread.id}`;
		openUrl(url, isCtrlClick);
	}
</script>

<Layout title="Task Runs">
	<div
		class="h-full w-full"
		in:fly={{ x: 100, duration: 300, delay: 150 }}
		out:fly={{ x: -100, duration: 300 }}
	>
		<div class="flex flex-col gap-8 pb-8">
			<div class="flex flex-col gap-2">
				<Search
					value={query}
					class="dark:bg-surface1 dark:border-surface3 bg-background border border-transparent shadow-sm"
					onChange={updateQuery}
					placeholder="Search threads..."
				/>

				{#if loading}
					<div class="flex w-full justify-center py-12">
						<LoaderCircle class="text-primary size-8 animate-spin" />
					</div>
				{:else if filteredThreads.length === 0}
					<div class="flex w-full flex-col items-center justify-center py-12 text-center">
						<MessageCircle class="text-on-surface1 size-24 opacity-50" />
						<h3 class="text-on-surface1 mt-4 text-lg font-semibold">No task runs available</h3>
						<p class="text-on-surface1 mt-2 text-sm font-light">
							Task runs will appear here once they are created.
						</p>
					</div>
				{:else}
					<Table
						data={tableData}
						fields={['name', 'userName', 'userEmail', 'task', 'projectName', 'created']}
						filterable={['name', 'userName', 'userEmail', 'task', 'projectName']}
						onFilter={setFilterUrlParams}
						filters={convertedUrlFilters}
						onClearAllFilters={clearUrlParams}
						onClickRow={isAuditor ? handleViewThread : undefined}
						headers={[
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
							}
						]}
						headerClasses={[
							{
								property: 'name',
								class: 'w-4/12 min-w-sm'
							}
						]}
						sortable={['name', 'userName', 'userEmail', 'projectName', 'created', 'task']}
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
										? 'View Task Run'
										: 'To view details, auditing permissions are required.'
								}}
								disabled={!isAuditor}
							>
								<Eye class="size-4" />
							</button>
						{/snippet}
						{#snippet onRenderColumn(property, thread)}
							{#if property === 'name'}
								<span>{thread.name || 'Unnamed Task Run'}</span>
							{:else if property === 'created'}
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
	<title>Obot | Admin - Task Runs</title>
</svelte:head>

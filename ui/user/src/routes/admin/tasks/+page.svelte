<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import {
		AdminService,
		type ProjectThread,
		type Project,
		type OrgUser,
		type ProjectTask,
		Group
	} from '$lib/services';
	import { Eye, LoaderCircle, MessageCircle } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';
	import { goto, replaceState } from '$app/navigation';
	import { formatTimeAgo } from '$lib/time';
	import Search from '$lib/components/Search.svelte';
	import { page } from '$app/state';
	import { debounce } from 'es-toolkit';
	import { profile } from '$lib/stores';
	import { twMerge } from 'tailwind-merge';
	import { tooltip } from '$lib/actions/tooltip.svelte';

	let tasks = $state<ProjectTask[]>([]);
	let threads = $state<ProjectThread[]>([]);
	let projects = $state<Project[]>([]);
	let users = $state<OrgUser[]>([]);
	let projectMap = $derived(new Map(projects.map((p) => [p.id, p])));
	let userMap = $derived(new Map(users.map((u) => [u.id, u])));

	let urlFilters = $state<Record<string, (string | number)[]>>({});
	let query = $state(page.url.searchParams.get('query') || '');

	let loading = $state(true);

	let taskRunsCount = $derived(
		threads.reduce<Record<string, number>>((acc, thread) => {
			acc[thread.taskID || ''] = (acc[thread.taskID || ''] || 0) + 1;
			return acc;
		}, {})
	);

	let tableData = $derived(
		tasks.map((task) => {
			const project = projectMap.get(task.projectID || '');
			return {
				...task,
				projectName: project?.name || task.projectID,
				userName: userMap.get(project?.userID || '')?.displayName || '-',
				userEmail: userMap.get(project?.userID || '')?.email || '-',
				runs: taskRunsCount[task.id] || 0
			};
		})
	);

	let isAuditor = $derived(profile.current.groups.includes(Group.AUDITOR));

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
			const tasksPromise = AdminService.listTasks().catch((err) => {
				console.error('Failed to load tasks', err);
				return [];
			});
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

			const [tasksData, threadsData, projectsData, usersData] = await Promise.race([
				Promise.all([tasksPromise, threadsPromise, projectsPromise, usersPromise]),
				timeoutPromise
			]);

			tasks = tasksData;
			projects = projectsData;
			users = usersData;
			threads = threadsData.filter((thread) => !!thread.taskRunID);
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

	function handleColumnFilter(property: string, values: string[]) {
		if (values.length === 0) {
			page.url.searchParams.delete(property);
		} else {
			page.url.searchParams.set(property, values.join(','));
		}

		replaceState(page.url, {});
	}

	function handleViewTask(task: ProjectTask) {
		goto(`/admin/tasks/${task.id}`);
	}
</script>

<Layout>
	<div
		class="my-4 h-full w-full"
		in:fly={{ x: 100, duration: 300, delay: 150 }}
		out:fly={{ x: -100, duration: 300 }}
	>
		<div class="flex flex-col gap-8 pb-8">
			<h1 class="text-2xl font-semibold">Tasks</h1>

			<div class="flex flex-col gap-2">
				<Search
					value={query}
					class="dark:bg-surface1 dark:border-surface3 border border-transparent bg-white shadow-sm"
					onChange={updateQuery}
					placeholder="Search threads..."
				/>

				{#if loading}
					<div class="flex w-full justify-center py-12">
						<LoaderCircle class="size-8 animate-spin text-blue-600" />
					</div>
				{:else if tasks.length === 0}
					<div class="flex w-full flex-col items-center justify-center py-12 text-center">
						<MessageCircle class="size-24 text-gray-200 dark:text-gray-700" />
						<h3 class="mt-4 text-lg font-semibold text-gray-400 dark:text-gray-600">
							No task available
						</h3>
						<p class="mt-2 text-sm font-light text-gray-400 dark:text-gray-600">
							Task will appear here once they are created.
						</p>
					</div>
				{:else}
					<Table
						data={tableData}
						fields={['name', 'userName', 'userEmail', 'projectName', 'created', 'runs']}
						filterable={['name', 'userName', 'userEmail', 'projectName']}
						onFilter={handleColumnFilter}
						filters={urlFilters}
						onClickRow={handleViewTask}
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
						sortable={['name', 'userName', 'userEmail', 'projectName', 'created', 'runs']}
						initSort={{ property: 'created', order: 'desc' }}
					>
						{#snippet actions(task)}
							<button
								class={twMerge('icon-button hover:text-blue-500')}
								onclick={(e) => {
									e.stopPropagation();
									handleViewTask(task);
								}}
								title="View Task"
								use:tooltip={{
									text: 'View Task'
								}}
							>
								<Eye class="size-4" />
							</button>
						{/snippet}
						{#snippet onRenderColumn(property, task)}
							{#if property === 'name'}
								<span>{task.name || 'Unnamed Task'}</span>
							{:else if property === 'created'}
								<span class="text-sm text-gray-600 dark:text-gray-400">
									{formatTimeAgo(task.created).relativeTime}
								</span>
							{:else if property === 'runs'}
								<a
									onclick={(e) => e.stopPropagation()}
									href={`/admin/task-runs?task=${task.id}`}
									class="text-sm font-semibold text-blue-500 hover:underline"
								>
									{taskRunsCount[task.id] || 0}
									{taskRunsCount[task.id] === 1 ? 'Run' : 'Runs'}
								</a>
							{:else}
								{task[property as keyof typeof task]}
							{/if}
						{/snippet}
					</Table>
				{/if}
			</div>
		</div>
	</div>
</Layout>

<svelte:head>
	<title>Obot | Admin - Tasks</title>
</svelte:head>

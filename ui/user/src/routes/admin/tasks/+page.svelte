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
	import { Eye, LoaderCircle, MessageCircle, Funnel } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';
	import { goto, replaceState } from '$app/navigation';
	import { formatTimeAgo } from '$lib/time';
	import Search from '$lib/components/Search.svelte';
	import { clickOutside } from '$lib/actions/clickoutside';
	import { dialogAnimation } from '$lib/actions/dialogAnimation';
	import { page } from '$app/state';
	import FiltersDrawer from '$lib/components/admin/filters-drawer/FiltersDrawer.svelte';
	import { getUserDisplayName } from '$lib/utils';
	import type { FilterOptionsEndpoint } from '$lib/components/admin/filters-drawer/types';
	import { debounce } from 'es-toolkit';
	import { profile } from '$lib/stores';
	import { twMerge } from 'tailwind-merge';
	import { tooltip } from '$lib/actions/tooltip.svelte';

	type SupportedFilter = 'username' | 'email' | 'project' | 'query';

	let tasks = $state<ProjectTask[]>([]);
	let threads = $state<ProjectThread[]>([]);
	let filteredTasks = $state<ProjectTask[]>([]);
	let projects = $state<Project[]>([]);
	let users = $state<OrgUser[]>([]);
	let projectMap = $derived(new Map(projects.map((p) => [p.id, p])));
	let userMap = $derived(new Map(users.map((u) => [u.id, u])));

	let rightSidebar = $state<HTMLDialogElement>();

	let showFilters = $state(false);

	const supportedFilters: Exclude<SupportedFilter, 'query'>[] = ['username', 'email', 'project'];

	const searchParamsAsArray: [SupportedFilter, string | undefined | null][] = $derived(
		supportedFilters.map((d) => {
			const hasSearchParam = page.url.searchParams.has(d);

			const value = page.url.searchParams.get(d);
			const isValueDefined = isSafe(value);

			return [
				d,
				isValueDefined
					? // Value is defined then decode and use it
						decodeURIComponent(value)
					: hasSearchParam
						? // Value is not defined but has a search param then override with empty string
							''
						: // No search params return default value if exist otherwise return undefined
							null
			];
		})
	);

	// Extract search supported params from the URL and convert them to AuditLogURLFilters
	// This is used to filter the audit logs based on the URL parameters
	const searchParamFilters = $derived.by<Record<SupportedFilter, string | undefined | null>>(() => {
		return searchParamsAsArray.reduce(
			(acc, [key, value]) => {
				acc[key!] = value;
				return acc;
			},
			{} as Record<SupportedFilter, string | undefined | null>
		);
	});

	let query = $state(page.url.searchParams.get('query') || '');

	// Base filters with time filters and query and pagination
	const pageFilters = $derived({
		...searchParamFilters,
		query
	});

	const options = $derived.by(() => {
		const usernameOptions = new Set<string>();
		const emailOptions = new Set<string>();
		const projectOptions = new Set<string>();

		tasks.forEach((task) => {
			const project = projects.find((p) => p.id === task.projectID);
			const user = userMap.get(project?.userID || '');

			if (user?.displayName) {
				usernameOptions.add(user.displayName);
			}

			if (user?.email) {
				emailOptions.add(user.email);
			}

			if (task.projectID) {
				projectOptions.add(task.projectID);
			}
		});

		return {
			username: { options: Array.from(usernameOptions).sort() },
			email: { options: Array.from(emailOptions).sort() },
			project: { options: Array.from(projectOptions).sort() }
		};
	});

	let loading = $state(true);

	let taskRunsCount = $derived(
		threads.reduce<Record<string, number>>((acc, thread) => {
			acc[thread.taskID || ''] = (acc[thread.taskID || ''] || 0) + 1;
			return acc;
		}, {})
	);

	let tableData = $derived(
		filteredTasks.map((task) => {
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
	});

	$effect(() => {
		filteredTasks = applyFilters(tasks, pageFilters);
	});

	function getFilterDisplayLabel(key: string) {
		if (key === 'email') return 'Email';
		if (key === 'project') return 'Project';
		if (key === 'username') return 'User Name';

		return key.replace(/_(\w)/g, ' $1');
	}

	function isSafe<T = unknown>(value: T) {
		return value !== undefined && value !== null;
	}

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

	function applyFilters(data: ProjectTask[] = tasks, filters: typeof pageFilters = pageFilters) {
		let filtered = [...data];

		type FilterFunction = [string | undefined | null, (array: ProjectTask[]) => ProjectTask[]];

		const queryFilterFunction = (array: ProjectTask[]) => {
			const lowercasedQuery = query.toLowerCase();
			return array.filter((task) => {
				const project = projectMap.get(task.projectID || '');
				const user = userMap.get(project?.userID || '');
				return (
					task.name?.toLowerCase().includes(lowercasedQuery) ||
					task.id.toLowerCase().includes(lowercasedQuery) ||
					project?.userID?.toLowerCase().includes(lowercasedQuery) ||
					project?.id?.toLowerCase().includes(lowercasedQuery) ||
					user?.displayName?.toLowerCase().includes(lowercasedQuery) ||
					user?.email?.toLowerCase().includes(lowercasedQuery) ||
					project?.name?.toLowerCase().includes(lowercasedQuery)
				);
			});
		};

		const usernameFilterFunction = (array: ProjectTask[]) => {
			return array.filter((task) => {
				const project = projectMap.get(task.projectID || '');
				const user = userMap.get(project?.userID || '');
				return (filters?.username ?? '')
					?.toLowerCase()
					.includes(user?.displayName?.toLowerCase() || '');
			});
		};

		const emailFilterFunction = (array: ProjectTask[]) => {
			return array.filter((task) => {
				const project = projectMap.get(task.projectID || '');
				const user = userMap.get(project?.userID || '');
				return (filters?.email ?? '')?.toLowerCase().includes(user?.email?.toLowerCase() || '');
			});
		};

		const projectFilterFunction = (array: ProjectTask[]) => {
			return array.filter((task) => {
				const project = projectMap.get(task.projectID || '');
				return (filters?.project ?? '')?.toLowerCase().includes(project?.id?.toLowerCase() || '');
			});
		};

		const filterFns: FilterFunction[] = [
			[pageFilters.query, queryFilterFunction],
			[filters.username, usernameFilterFunction],
			[filters.email, emailFilterFunction],
			[filters.project, projectFilterFunction]
		].filter((d) => !!d[0]) as FilterFunction[];

		// sort by most recent
		return filterFns
			.reduce((acc, val) => val[1](acc), filtered)
			.sort((a, b) => new Date(b.created).getTime() - new Date(a.created).getTime());
	}

	function handleViewTask(task: ProjectTask) {
		goto(`/admin/tasks/${task.id}`);
	}

	function handleRightSidebarClose() {
		rightSidebar?.close();
		setTimeout(() => {
			showFilters = false;
		}, 300);
	}

	async function optionsEndpoint(filterId: SupportedFilter) {
		switch (filterId) {
			case 'username':
				return options.username;
			case 'email':
				return options.email;
			case 'project':
				return options.project;
			default:
				return [];
		}
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
				<div class="flex items-center gap-4">
					<Search
						value={query}
						class="dark:bg-surface1 dark:border-surface3 border border-transparent bg-white shadow-sm"
						onChange={updateQuery}
						placeholder="Search threads..."
					/>
					<button
						class="hover:bg-surface1 dark:bg-surface1 dark:hover:bg-surface3 dark:border-surface3 button flex h-12 w-fit items-center justify-center gap-1 rounded-lg border border-transparent bg-white shadow-sm"
						onclick={() => {
							showFilters = true;
							rightSidebar?.show();
						}}
					>
						<Funnel class="size-4" />
						Filters
					</button>
				</div>

				{#if loading}
					<div class="flex w-full justify-center py-12">
						<LoaderCircle class="size-8 animate-spin text-blue-600" />
					</div>
				{:else if filteredTasks.length === 0}
					<div class="flex w-full flex-col items-center justify-center py-12 text-center">
						<MessageCircle class="size-24 text-gray-200 dark:text-gray-700" />
						<h3 class="mt-4 text-lg font-semibold text-gray-400 dark:text-gray-600">
							{#if query}
								No tasks found
							{:else}
								No task available
							{/if}
						</h3>
						<p class="mt-2 text-sm font-light text-gray-400 dark:text-gray-600">
							{#if query}
								Try adjusting your search terms.
							{:else}
								Task will appear here once they are created.
							{/if}
						</p>
					</div>
				{:else}
					<Table
						data={tableData}
						fields={['name', 'userName', 'userEmail', 'projectName', 'created', 'runs']}
						onSelectRow={isAuditor ? handleViewTask : undefined}
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
					>
						{#snippet actions(task)}
							<button
								class={twMerge(
									'icon-button',
									isAuditor && 'hover:text-blue-500',
									!isAuditor && 'opacity-50 hover:bg-transparent dark:hover:bg-transparent'
								)}
								onclick={(e) => {
									e.stopPropagation();
									handleViewTask(task);
								}}
								title="View Task"
								use:tooltip={{
									text: isAuditor
										? 'View Task'
										: 'To view details, auditing permissions are required.'
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

<dialog
	bind:this={rightSidebar}
	use:clickOutside={[handleRightSidebarClose, true]}
	use:dialogAnimation={{ type: 'drawer' }}
	class="dark:border-surface1 dark:bg-surface1 fixed! top-0! right-0! bottom-0! left-auto! z-40 h-screen w-auto max-w-none rounded-none border-0 bg-white shadow-lg outline-none!"
>
	{#if showFilters}
		<FiltersDrawer
			onClose={handleRightSidebarClose}
			filters={searchParamFilters}
			{getFilterDisplayLabel}
			getUserDisplayName={(...args) => getUserDisplayName(userMap, ...args)}
			endpoint={optionsEndpoint as unknown as FilterOptionsEndpoint}
		/>
	{/if}
</dialog>

<svelte:head>
	<title>Obot | Admin - Tasks</title>
</svelte:head>

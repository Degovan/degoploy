<script lang="ts">
	import { createQuery } from '@tanstack/svelte-query';
	import { resolve } from '$app/paths';
	import { projectKeys, projectsApi } from '#lib/api/projects.js';
	import { shortRef } from '#lib/api/deployments.js';
	import type { Project } from '#lib/api/types.gen.js';
	import PageHeader from '#lib/components/page-header.svelte';
	import ProjectCard from '#lib/components/project-card.svelte';
	import ProjectAvatar from '#lib/components/project-avatar.svelte';
	import ProviderIcon from '#lib/components/provider-icon.svelte';
	import RecentDeployments from '#lib/components/recent-deployments.svelte';
	import StatusDot from '#lib/components/status-dot.svelte';
	import HealthIcon, { healthPercent, healthSummary } from '#lib/components/health-icon.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import * as Alert from '#lib/components/ui/alert/index.js';
	import { session } from '#lib/stores/session.svelte.js';
	import { parseRepo } from '#lib/repo.js';
	import { formatDateTime, shortAgo } from '#lib/time.js';
	import { cn } from '#lib/utils.js';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import LayoutGridIcon from '@lucide/svelte/icons/layout-grid';
	import ListIcon from '@lucide/svelte/icons/list';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import SearchIcon from '@lucide/svelte/icons/search';

	const projects = createQuery(() => ({
		queryKey: projectKeys.all,
		queryFn: projectsApi.list
	}));

	let query = $state('');
	let searchInput = $state<HTMLInputElement | null>(null);

	// Grid or list, remembered per browser.
	const VIEW_KEY = 'degoploy.projects.view';
	let view = $state<'grid' | 'list'>(readView());
	function readView(): 'grid' | 'list' {
		try {
			return localStorage.getItem(VIEW_KEY) === 'list' ? 'list' : 'grid';
		} catch {
			return 'grid';
		}
	}
	function setView(v: 'grid' | 'list') {
		view = v;
		try {
			localStorage.setItem(VIEW_KEY, v);
		} catch {
			// Storage may be unavailable (private mode); the choice just isn't remembered.
		}
	}

	const filtered = $derived.by(() => {
		const q = query.trim().toLowerCase();
		const list = projects.data ?? [];
		if (!q) return list;
		return list.filter((p) =>
			[p.name, p.slug, p.repo_url, p.work_dir].some((s) => s.toLowerCase().includes(q))
		);
	});

	type Bucket = 'healthy' | 'failing' | 'running' | null;
	function bucket(p: Project): Bucket {
		const s = p.last_deployment?.status;
		if (!s) return null;
		if (s === 'running' || s === 'queued') return 'running';
		if (s === 'failed' || s === 'timeout') return 'failing';
		if (s === 'success') return 'healthy';
		// Canceled or skipped says nothing about whether the project deploys cleanly.
		return null;
	}
	const summary = $derived.by(() => {
		const counts = { healthy: 0, failing: 0, running: 0 };
		for (const p of projects.data ?? []) {
			const b = bucket(p);
			if (b) counts[b]++;
		}
		return counts;
	});

	let now = $state(Date.now());
	$effect(() => {
		const t = setInterval(() => (now = Date.now()), 30_000);
		return () => clearInterval(t);
	});

	// "/" focuses the search box, unless the user is already typing somewhere.
	function onKeydown(e: KeyboardEvent) {
		if (e.key !== '/' || e.metaKey || e.ctrlKey || e.altKey) return;
		const el = e.target as HTMLElement;
		if (el.closest('input, textarea, select, [contenteditable="true"]')) return;
		e.preventDefault();
		searchInput?.focus();
	}
</script>

<svelte:window onkeydown={onKeydown} />

<PageHeader
	title="Projects"
	description="Repositories that deploy automatically when a webhook arrives."
>
	{#snippet aside()}
		{#if projects.data?.length}
			<div class="flex flex-wrap items-center gap-x-5 gap-y-1 text-[13px] text-muted-foreground">
				<span class="flex items-center gap-2">
					<span class="size-2 rounded-full bg-emerald-400"></span>{summary.healthy} healthy
				</span>
				<span class="flex items-center gap-2">
					<span class="size-2 rounded-full bg-red-500"></span>{summary.failing} failing
				</span>
				<span class="flex items-center gap-2">
					<span class="size-2 rounded-full bg-amber-400"></span>{summary.running} running
				</span>
			</div>
		{/if}
	{/snippet}
</PageHeader>

<div class="container-page flex flex-col gap-12 py-10">
	<div class="flex flex-col gap-6">
		<div class="flex flex-wrap items-center gap-3">
			<label class="relative min-w-60 flex-1">
				<span class="sr-only">Search projects</span>
				<SearchIcon
					class="pointer-events-none absolute top-1/2 left-3.5 size-4 -translate-y-1/2 text-muted-foreground"
				/>
				<input
					bind:this={searchInput}
					bind:value={query}
					type="search"
					placeholder="Search Projects…"
					class="h-11 w-full rounded-lg border border-input bg-card pr-12 pl-10 text-sm outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/40"
				/>
				<kbd
					class="pointer-events-none absolute top-1/2 right-3 -translate-y-1/2 rounded border bg-muted px-1.5 font-mono text-xs text-muted-foreground"
					>/</kbd
				>
			</label>
			<div
				class="flex h-11 items-center gap-1 rounded-lg border bg-card p-1"
				role="group"
				aria-label="View"
			>
				{#each [['grid', LayoutGridIcon, 'Grid view'], ['list', ListIcon, 'List view']] as const as [v, Icon, label] (v)}
					<button
						type="button"
						class={cn(
							'flex size-9 items-center justify-center rounded-md text-muted-foreground transition-colors hover:text-foreground',
							view === v && 'bg-accent text-foreground'
						)}
						aria-label={label}
						aria-pressed={view === v}
						onclick={() => setView(v)}
					>
						<Icon class="size-4" />
					</button>
				{/each}
			</div>
			{#if session.can('project.create')}
				<Button href={resolve('/(app)/projects/new')} class="h-11 px-4 text-[13px]">
					<PlusIcon />
					New Project
				</Button>
			{/if}
		</div>

		{#if projects.isPending}
			<div class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
				{#each [1, 2, 3] as i (i)}
					<Skeleton class="h-52 rounded-xl" />
				{/each}
			</div>
		{:else if projects.isError}
			<Alert.Root variant="destructive">
				<CircleAlertIcon />
				<Alert.Title>{projects.error.message}</Alert.Title>
			</Alert.Root>
		{:else if filtered.length === 0 && query}
			<div class="rounded-xl border border-dashed p-10 text-center text-sm text-muted-foreground">
				No projects match “{query}”.
			</div>
		{:else if view === 'grid'}
			<div class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
				{#each filtered as p (p.id)}
					<ProjectCard project={p} {now} />
				{/each}
				{#if session.can('project.create') && !query}
					<a
						href={resolve('/(app)/projects/new')}
						class="flex min-h-52 flex-col items-center justify-center gap-2 rounded-xl border border-dashed p-6 text-center transition-colors hover:border-foreground/30 hover:bg-card"
					>
						<span class="mb-2 flex size-10 items-center justify-center rounded-full border">
							<PlusIcon class="size-4" />
						</span>
						<span class="text-sm font-medium">New Project</span>
						<span class="text-[13px] text-muted-foreground"
							>Connect a GitHub or Gitea repository</span
						>
					</a>
				{/if}
			</div>
			{#if filtered.length === 0 && !session.can('project.create')}
				<p class="text-sm text-muted-foreground">No projects yet. Ask an admin to add one.</p>
			{/if}
		{:else}
			<ul class="divide-y overflow-hidden rounded-xl border bg-card">
				{#each filtered as p (p.id)}
					{@const d = p.last_deployment}
					<li>
						<a
							href={resolve('/(app)/projects/[id]', { id: String(p.id) })}
							class={cn(
								'grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-4 px-4 py-3 text-sm transition-colors hover:bg-accent/50 md:grid-cols-[auto_minmax(0,1fr)_minmax(0,16rem)_minmax(0,14rem)_4rem] md:px-6',
								!p.enabled && 'text-muted-foreground'
							)}
						>
							<ProjectAvatar seed={p.slug} muted={!p.enabled} class="size-7" />
							<span class="flex min-w-0 flex-col">
								<span class="truncate font-medium">
									{p.name}{#if !p.enabled}<span class="font-normal"> · Disabled</span>{/if}
								</span>
								<span class="truncate font-mono text-xs text-muted-foreground">{p.work_dir}</span>
							</span>
							<span class="hidden min-w-0 items-center gap-1.5 text-muted-foreground md:flex">
								<ProviderIcon provider={p.provider} class="size-3.5 shrink-0" />
								<span class="truncate">{parseRepo(p.repo_url)?.path ?? p.repo_url}</span>
							</span>
							<span class="hidden min-w-0 items-center gap-2 md:flex">
								{#if d}
									<StatusDot status={d.status} />
									<span class="truncate text-muted-foreground" title={formatDateTime(d.queued_at)}>
										#{d.id} · {shortAgo(d.queued_at, now)} · {shortRef(d.ref)}
									</span>
								{:else}
									<span class="text-muted-foreground">Never deployed</span>
								{/if}
							</span>
							<span
								class="flex items-center justify-end gap-1.5 font-mono text-xs text-muted-foreground"
								title={p.health ? healthSummary(p.health) : 'No finished deployments'}
							>
								{#if p.health}
									<HealthIcon health={p.health} />{healthPercent(p.health)}%
								{:else}—{/if}
							</span>
						</a>
					</li>
				{/each}
			</ul>
		{/if}
	</div>

	{#if projects.data && projects.data.length > 0}
		<RecentDeployments projects={projects.data} />
	{/if}
</div>

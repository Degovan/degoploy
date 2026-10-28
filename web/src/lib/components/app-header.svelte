<script lang="ts">
	import { createQuery } from '@tanstack/svelte-query';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { projectKeys, projectsApi } from '#lib/api/projects.js';
	import type { Action } from '#lib/api/types.gen.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import * as DropdownMenu from '#lib/components/ui/dropdown-menu/index.js';
	import Logo from '#lib/components/logo.svelte';
	import ProjectAvatar from '#lib/components/project-avatar.svelte';
	import UserMenu from '#lib/components/user-menu.svelte';
	import { realtime } from '#lib/realtime.svelte.js';
	import { session } from '#lib/stores/session.svelte.js';
	import { cn } from '#lib/utils.js';
	import ChevronsUpDownIcon from '@lucide/svelte/icons/chevrons-up-down';
	import CheckIcon from '@lucide/svelte/icons/check';
	import PlusIcon from '@lucide/svelte/icons/plus';

	const DOCS_URL = 'https://github.com/degovan/degoploy#readme';

	const projectId = $derived(page.params.id ? Number(page.params.id) : null);
	const deploymentId = $derived(page.params.did ? Number(page.params.did) : null);
	const isNewProject = $derived(page.route.id === '/(app)/projects/new');

	const projects = createQuery(() => ({
		queryKey: projectKeys.all,
		queryFn: projectsApi.list,
		enabled: projectId !== null
	}));
	const current = $derived(projects.data?.find((p) => p.id === projectId));

	type Tab = { label: string; href: string; exact?: boolean; requires?: Action };

	// hrefs are resolved from literal routes: resolve() cannot take a union of route ids.
	const tabs = $derived.by((): Tab[] => {
		if (projectId !== null) {
			const id = String(projectId);
			return [
				{ label: 'Overview', href: resolve('/(app)/projects/[id]', { id }), exact: true },
				{ label: 'Triggers', href: resolve('/(app)/projects/[id]/triggers', { id }) },
				{
					label: 'Script',
					href: resolve('/(app)/projects/[id]/script', { id }),
					requires: 'project.deploy'
				},
				{ label: 'Environment', href: resolve('/(app)/projects/[id]/environment', { id }) },
				{
					label: 'Settings',
					href: resolve('/(app)/projects/[id]/settings', { id }),
					requires: 'project.edit'
				}
			];
		}
		if (isNewProject) return [];
		return [
			{ label: 'Projects', href: resolve('/(app)/projects') },
			{ label: 'Users', href: resolve('/(app)/users'), requires: 'user.manage' },
			{ label: 'Audit log', href: resolve('/(app)/audit'), requires: 'audit.view' },
			{ label: 'SSO', href: resolve('/(app)/sso'), requires: 'user.manage' }
		];
	});
	const visibleTabs = $derived(tabs.filter((t) => !t.requires || session.can(t.requires)));

	function isActive(t: Tab) {
		const path = page.url.pathname;
		// The overview tab also covers a project's deployment pages.
		if (t.exact) return path === t.href || path.startsWith(t.href + '/deployments');
		return path === t.href || path.startsWith(t.href + '/');
	}

	const live = $derived(
		{
			open: { label: 'Live', dot: 'bg-emerald-400' },
			connecting: { label: 'Connecting…', dot: 'bg-amber-400 animate-pulse' },
			closed: { label: 'Reconnecting…', dot: 'bg-amber-400 animate-pulse' },
			idle: { label: 'Offline', dot: 'bg-muted-foreground' }
		}[realtime.status]
	);
</script>

{#snippet slash()}
	<svg viewBox="0 0 24 24" class="size-5 shrink-0 text-muted-foreground/40" aria-hidden="true">
		<path d="M16 3 8 21" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" />
	</svg>
{/snippet}

<header class="sticky top-0 z-40 border-b bg-background/80 backdrop-blur-md">
	<div class="flex h-16 items-center gap-2 px-4 md:px-6">
		<nav class="flex min-w-0 items-center gap-2" aria-label="Breadcrumb">
			<a
				href={resolve('/(app)/projects')}
				class="shrink-0 rounded-lg"
				aria-label="degoploy projects"
			>
				<Logo class="size-8" />
			</a>
			{@render slash()}
			<a
				href={resolve('/(app)/projects')}
				class="shrink-0 text-sm font-medium hover:text-foreground/80">degoploy</a
			>
			{#if projectId !== null}
				{@render slash()}
				<DropdownMenu.Root>
					<DropdownMenu.Trigger
						class="flex min-w-0 items-center gap-2 rounded-md px-1.5 py-1 text-sm font-medium hover:bg-accent"
						aria-label="Switch project"
					>
						{#if current}
							<ProjectAvatar seed={current.slug} muted={!current.enabled} class="size-5" />
							<span class="truncate">{current.name}</span>
						{:else}
							<span class="text-muted-foreground">Project</span>
						{/if}
						<ChevronsUpDownIcon class="size-3.5 shrink-0 text-muted-foreground" />
					</DropdownMenu.Trigger>
					<DropdownMenu.Content align="start" class="w-64">
						<DropdownMenu.Label class="text-xs text-muted-foreground">Projects</DropdownMenu.Label>
						{#each projects.data ?? [] as p (p.id)}
							<DropdownMenu.Item>
								{#snippet child({ props })}
									<a href={resolve('/(app)/projects/[id]', { id: String(p.id) })} {...props}>
										<ProjectAvatar seed={p.slug} muted={!p.enabled} class="size-4" />
										<span class="truncate">{p.name}</span>
										{#if p.id === projectId}<CheckIcon class="ml-auto" />{/if}
									</a>
								{/snippet}
							</DropdownMenu.Item>
						{/each}
						{#if session.can('project.create')}
							<DropdownMenu.Separator />
							<DropdownMenu.Item>
								{#snippet child({ props })}
									<a href={resolve('/(app)/projects/new')} {...props}>
										<PlusIcon />
										New Project
									</a>
								{/snippet}
							</DropdownMenu.Item>
						{/if}
					</DropdownMenu.Content>
				</DropdownMenu.Root>
				{#if deploymentId !== null}
					{@render slash()}
					<span class="shrink-0 font-mono text-sm font-medium">#{deploymentId}</span>
				{/if}
			{:else if isNewProject}
				{@render slash()}
				<span class="truncate text-sm font-medium">New Project</span>
			{/if}
		</nav>

		<div class="ml-auto flex shrink-0 items-center gap-3">
			<span
				class="hidden items-center gap-2 text-[13px] text-muted-foreground sm:flex"
				title="Realtime connection"
				data-testid="realtime-status"
			>
				<span class={cn('size-2 rounded-full', live.dot)}></span>
				{live.label}
			</span>
			<Button
				variant="outline"
				size="sm"
				class="hidden text-[13px] font-normal sm:inline-flex"
				href={DOCS_URL}
				target="_blank"
				rel="noreferrer"
			>
				Docs
			</Button>
			<UserMenu />
		</div>
	</div>

	{#if visibleTabs.length}
		<nav class="-mb-px flex gap-1 overflow-x-auto px-2 md:px-4" aria-label="Sections">
			{#each visibleTabs as tab (tab.href)}
				{@const active = isActive(tab)}
				<!-- eslint-disable svelte/no-navigation-without-resolve -- resolved in tabs -->
				<a
					href={tab.href}
					class={cn(
						'relative px-3 py-3 text-sm whitespace-nowrap transition-colors after:absolute after:inset-x-2 after:bottom-0 after:h-0.5 after:rounded-full',
						active
							? 'text-foreground after:bg-foreground'
							: 'text-muted-foreground hover:text-foreground'
					)}
					aria-current={active ? 'page' : undefined}
				>
					{tab.label}
				</a>
				<!-- eslint-enable svelte/no-navigation-without-resolve -->
			{/each}
		</nav>
	{/if}
</header>

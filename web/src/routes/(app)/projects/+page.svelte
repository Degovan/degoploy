<script lang="ts">
	import { createQuery } from '@tanstack/svelte-query';
	import { resolve } from '$app/paths';
	import { projectKeys, projectsApi } from '#lib/api/projects.js';
	import PageHeader from '#lib/components/page-header.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Badge } from '#lib/components/ui/badge/index.js';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import * as Card from '#lib/components/ui/card/index.js';
	import * as Alert from '#lib/components/ui/alert/index.js';
	import { session } from '#lib/stores/session.svelte.js';
	import { parseRepo, providerLabel } from '#lib/repo.js';
	import DeployStatusBadge from '#lib/components/deploy-status-badge.svelte';
	import { timeAgo } from '#lib/time.js';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import FolderGitIcon from '@lucide/svelte/icons/folder-git-2';
	import FolderIcon from '@lucide/svelte/icons/folder';
	import PlusIcon from '@lucide/svelte/icons/plus';

	const projects = createQuery(() => ({
		queryKey: projectKeys.all,
		queryFn: projectsApi.list
	}));
</script>

<PageHeader title="Projects" description="Repositories that deploy when a webhook arrives.">
	{#snippet actions()}
		{#if session.can('project.create')}
			<Button href={resolve('/(app)/projects/new')}>
				<PlusIcon />
				New project
			</Button>
		{/if}
	{/snippet}
</PageHeader>

{#if projects.isPending}
	<div class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
		{#each [1, 2, 3] as i (i)}
			<Skeleton class="h-36 rounded-xl" />
		{/each}
	</div>
{:else if projects.isError}
	<Alert.Root variant="destructive">
		<CircleAlertIcon />
		<Alert.Title>{projects.error.message}</Alert.Title>
	</Alert.Root>
{:else if projects.data.length === 0}
	<Card.Root class="border-dashed">
		<Card.Content class="flex flex-col items-center gap-3 py-12 text-center">
			<div class="rounded-full bg-muted p-3 text-muted-foreground">
				<FolderGitIcon class="size-6" />
			</div>
			<div class="flex flex-col gap-1">
				<p class="font-medium">No projects yet</p>
				<p class="text-sm text-muted-foreground">
					Add a repository, then point its GitHub or Gitea webhook at degoploy.
				</p>
			</div>
			{#if session.can('project.create')}
				<Button href={resolve('/(app)/projects/new')} variant="outline" class="mt-2">
					<PlusIcon />
					New project
				</Button>
			{/if}
		</Card.Content>
	</Card.Root>
{:else}
	<div class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
		{#each projects.data as p (p.id)}
			<a
				href={resolve('/(app)/projects/[id]', { id: String(p.id) })}
				class="group rounded-xl focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none"
			>
				<Card.Root class="h-full transition-colors group-hover:border-foreground/20">
					<Card.Header>
						<Card.Title class="flex items-center gap-2">
							<span class="truncate">{p.name}</span>
							{#if !p.enabled}<Badge variant="secondary">Disabled</Badge>{/if}
						</Card.Title>
						<Card.Description class="truncate font-mono text-xs">
							{parseRepo(p.repo_url)?.path ?? p.repo_url}
						</Card.Description>
					</Card.Header>
					<Card.Content class="flex flex-col gap-3 text-sm">
						<div class="flex items-center gap-2 text-muted-foreground">
							<FolderIcon class="size-4 shrink-0" />
							<span class="truncate font-mono text-xs">{p.work_dir}</span>
						</div>
						{#if p.last_deployment}
							<div class="flex items-center gap-2 text-xs text-muted-foreground">
								<DeployStatusBadge status={p.last_deployment.status} />
								#{p.last_deployment.id} · {timeAgo(p.last_deployment.queued_at)}
							</div>
						{:else}
							<p class="text-xs text-muted-foreground">Never deployed</p>
						{/if}
						<div class="flex flex-wrap gap-1.5">
							<Badge variant="outline">{providerLabel[p.provider]}</Badge>
							<Badge variant="outline">
								{p.script_source === 'managed' ? 'Managed script' : 'Repo script'}
							</Badge>
						</div>
					</Card.Content>
				</Card.Root>
			</a>
		{/each}
	</div>
{/if}

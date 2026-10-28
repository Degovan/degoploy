<script lang="ts">
	import { createQuery } from '@tanstack/svelte-query';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { ApiError } from '#lib/api/client.js';
	import { projectKeys, projectsApi } from '#lib/api/projects.js';
	import { ruleKeys, rulesApi } from '#lib/api/rules.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import * as Alert from '#lib/components/ui/alert/index.js';
	import { session } from '#lib/stores/session.svelte.js';
	import DeployButton from '#lib/components/deploy-button.svelte';
	import ProviderIcon from '#lib/components/provider-icon.svelte';
	import { parseRepo, providerLinks } from '#lib/repo.js';
	import ArrowLeftIcon from '@lucide/svelte/icons/arrow-left';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import FileCodeIcon from '@lucide/svelte/icons/file-code';
	import FolderIcon from '@lucide/svelte/icons/folder';
	import GitBranchIcon from '@lucide/svelte/icons/git-branch';
	import PauseIcon from '@lucide/svelte/icons/pause';
	import type { LayoutProps } from './$types';

	let { data, children }: LayoutProps = $props();

	const project = createQuery(() => ({
		queryKey: projectKeys.detail(data.id),
		queryFn: () => projectsApi.get(data.id)
	}));
	const rules = createQuery(() => ({
		queryKey: ruleKeys.list(data.id),
		queryFn: () => rulesApi.list(data.id)
	}));

	// Branches the enabled push rules deploy, e.g. "main" or "main, release/*".
	const branches = $derived(
		(rules.data ?? [])
			.filter((r) => r.enabled && r.event === 'push')
			.map((r) => r.ref_pattern)
			.join(', ')
	);
	const links = $derived(
		project.data ? providerLinks(project.data.provider, project.data.repo_url) : null
	);
	// These pages render their own header band and body instead of the project hero.
	const ownLayouts = new Set([
		'/(app)/projects/[id]/deployments/[did]',
		'/(app)/projects/[id]/triggers',
		'/(app)/projects/[id]/script'
	]);
	const ownLayout = $derived(ownLayouts.has(page.route.id ?? ''));
</script>

<svelte:head>
	<title>{project.data ? `${project.data.name} · degoploy` : 'Project · degoploy'}</title>
</svelte:head>

{#if project.isPending}
	<div class="container-page flex flex-col gap-4 py-10">
		<Skeleton class="h-10 w-64" />
		<Skeleton class="h-64 w-full rounded-xl" />
	</div>
{:else if project.isError}
	<div class="container-page flex flex-col gap-4 py-10">
		<Alert.Root variant="destructive">
			<CircleAlertIcon />
			<Alert.Title>
				{project.error instanceof ApiError && project.error.status === 404
					? 'Project not found'
					: project.error.message}
			</Alert.Title>
		</Alert.Root>
		<div>
			<Button variant="outline" href={resolve('/(app)/projects')}>
				<ArrowLeftIcon />
				All projects
			</Button>
		</div>
	</div>
{:else if ownLayout}
	{@render children()}
{:else}
	{@const p = project.data}
	<section class="border-b">
		<div
			class="container-page flex flex-wrap items-start justify-between gap-6 py-10 md:flex-nowrap"
		>
			<div class="flex min-w-0 flex-1 flex-col gap-3">
				<h1 class="text-3xl font-semibold tracking-tight md:text-4xl">{p.name}</h1>
				<div class="flex flex-wrap items-center gap-x-4 gap-y-2 text-sm text-muted-foreground">
					<span
						class="inline-flex h-7 max-w-full items-center gap-1.5 rounded-full bg-muted px-3 text-foreground"
					>
						<ProviderIcon provider={p.provider} class="size-3.5 shrink-0 text-muted-foreground" />
						<span class="truncate">{parseRepo(p.repo_url)?.path ?? p.repo_url}</span>
					</span>
					{#if branches}
						<span class="flex min-w-0 items-center gap-1.5 font-mono">
							<GitBranchIcon class="size-4 shrink-0" />
							<span class="truncate">{branches}</span>
						</span>
					{/if}
					<span class="flex min-w-0 items-center gap-1.5 font-mono">
						<FolderIcon class="size-4 shrink-0" />
						<span class="truncate">{p.work_dir}</span>
					</span>
					<span class="flex items-center gap-1.5">
						<FileCodeIcon class="size-4 shrink-0" />
						{#if p.script_source === 'managed'}Managed script{:else}<span class="font-mono"
								>{p.script_path}</span
							>{/if}
					</span>
					{#if !p.enabled}
						<span
							class="inline-flex h-7 items-center gap-1.5 rounded-full border px-3 text-foreground"
						>
							<PauseIcon class="size-3.5" />
							Disabled
						</span>
					{/if}
				</div>
			</div>
			<div class="flex shrink-0 items-center gap-2">
				{#if links}
					<Button variant="outline" href={links.repo} target="_blank" rel="noreferrer">
						<ProviderIcon provider={p.provider} />
						Repository
					</Button>
				{/if}
				{#if session.can('project.deploy')}
					<DeployButton projectId={p.id} />
				{/if}
			</div>
		</div>
	</section>

	<div class="container-page flex flex-col gap-12 py-10">
		{@render children()}
	</div>
{/if}

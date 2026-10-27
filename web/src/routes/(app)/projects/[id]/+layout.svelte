<script lang="ts">
	import { createQuery } from '@tanstack/svelte-query';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { ApiError } from '#lib/api/client.js';
	import { projectKeys, projectsApi } from '#lib/api/projects.js';
	import { Badge } from '#lib/components/ui/badge/index.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import * as Alert from '#lib/components/ui/alert/index.js';
	import { session } from '#lib/stores/session.svelte.js';
	import DeployButton from '#lib/components/deploy-button.svelte';
	import { providerLabel } from '#lib/repo.js';
	import { cn } from '#lib/utils.js';
	import ArrowLeftIcon from '@lucide/svelte/icons/arrow-left';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import type { LayoutProps } from './$types';

	let { data, children }: LayoutProps = $props();

	const project = createQuery(() => ({
		queryKey: projectKeys.detail(data.id),
		queryFn: () => projectsApi.get(data.id)
	}));

	// hrefs are resolved from literal routes: resolve() cannot take a union of route ids.
	const tabs = $derived.by(() => {
		const id = String(data.id);
		return [
			{ label: 'Overview', href: resolve('/(app)/projects/[id]', { id }), show: true },
			{ label: 'Triggers', href: resolve('/(app)/projects/[id]/triggers', { id }), show: true },
			{
				label: 'Script',
				href: resolve('/(app)/projects/[id]/script', { id }),
				show: session.can('project.deploy')
			},
			{
				label: 'Settings',
				href: resolve('/(app)/projects/[id]/settings', { id }),
				show: session.can('project.edit')
			}
		];
	});
</script>

<svelte:head>
	<title>{project.data ? `${project.data.name} · degoploy` : 'Project · degoploy'}</title>
</svelte:head>

{#if project.isPending}
	<Skeleton class="h-10 w-64" />
	<Skeleton class="h-64 w-full rounded-xl" />
{:else if project.isError}
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
{:else}
	{@const p = project.data}

	<div class="flex flex-col gap-4">
		<a
			href={resolve('/(app)/projects')}
			class="flex w-fit items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
		>
			<ArrowLeftIcon class="size-4" />
			Projects
		</a>
		<div class="flex flex-wrap items-center gap-2">
			<h1 class="text-2xl font-semibold tracking-tight">{p.name}</h1>
			<Badge variant="outline">{providerLabel[p.provider]}</Badge>
			{#if !p.enabled}<Badge variant="secondary">Disabled</Badge>{/if}
			{#if session.can('project.deploy')}
				<div class="ml-auto"><DeployButton projectId={p.id} /></div>
			{/if}
		</div>
		<nav class="-mb-px flex gap-1 border-b">
			{#each tabs.filter((t) => t.show) as tab (tab.href)}
				{@const active = page.url.pathname === tab.href}
				<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- resolved in tabs -->
				<a
					href={tab.href}
					class={cn(
						'border-b-2 px-3 py-2 text-sm font-medium transition-colors',
						active
							? 'border-primary text-foreground'
							: 'border-transparent text-muted-foreground hover:text-foreground'
					)}
					aria-current={active ? 'page' : undefined}
				>
					{tab.label}
				</a>
			{/each}
		</nav>
	</div>

	{@render children()}
{/if}

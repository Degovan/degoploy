<script lang="ts">
	import { createQuery } from '@tanstack/svelte-query';
	import { resolve } from '$app/paths';
	import { deploymentKeys, deploymentsApi, shortRef } from '#lib/api/deployments.js';
	import type { Project } from '#lib/api/types.gen.js';
	import ProjectAvatar from '#lib/components/project-avatar.svelte';
	import SectionHeading from '#lib/components/section-heading.svelte';
	import StatusDot from '#lib/components/status-dot.svelte';
	import StatusIcon from '#lib/components/status-icon.svelte';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import { runMs } from '#lib/stages.js';
	import { formatDateTime, formatDuration, shortAgo } from '#lib/time.js';
	import GitBranchIcon from '@lucide/svelte/icons/git-branch';
	import RocketIcon from '@lucide/svelte/icons/rocket';

	let { projects }: { projects: Project[] } = $props();

	const recent = createQuery(() => ({
		queryKey: deploymentKeys.recent,
		queryFn: () => deploymentsApi.recent(8)
	}));

	const byId = $derived(new Map(projects.map((p) => [p.id, p])));

	const active = createQuery(() => ({
		queryKey: deploymentKeys.active,
		queryFn: deploymentsApi.active
	}));

	const queue = $derived.by(() => {
		const list = active.data ?? [];
		const queued = list.filter((d) => d.status === 'queued').length;
		const running = list.filter((d) => d.status === 'running').length;
		return [queued ? `${queued} queued` : 'Queue empty', running ? `${running} running` : '']
			.filter(Boolean)
			.join(' · ');
	});

	let now = $state(Date.now());
	$effect(() => {
		const t = setInterval(() => (now = Date.now()), 1000);
		return () => clearInterval(t);
	});
</script>

<section class="flex flex-col gap-4" aria-labelledby="recent-deployments">
	<SectionHeading title="Recent deployments" id="recent-deployments" size="sm">
		{#snippet actions()}
			{#if active.data}<span class="text-[13px] text-muted-foreground">{queue}</span>{/if}
		{/snippet}
	</SectionHeading>

	{#if recent.isPending}
		<Skeleton class="h-48 w-full rounded-xl" />
	{:else if recent.isError}
		<p class="text-sm text-destructive">{recent.error.message}</p>
	{:else if recent.data.length === 0}
		<div
			class="flex flex-col items-center gap-2 rounded-xl border border-dashed p-8 text-center text-sm text-muted-foreground"
		>
			<RocketIcon class="size-5" />
			Nothing deployed yet. Deployments from every project show up here.
		</div>
	{:else}
		<ul class="divide-y overflow-hidden rounded-xl border bg-card">
			{#each recent.data as d (d.id)}
				{@const p = byId.get(d.project_id)}
				{@const ms = runMs(d, now)}
				<li>
					<a
						href={resolve('/(app)/projects/[id]/deployments/[did]', {
							id: String(d.project_id),
							did: String(d.id)
						})}
						class="grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-x-4 gap-y-1 px-4 py-3 text-sm transition-colors hover:bg-accent/50 md:grid-cols-[auto_minmax(0,14rem)_minmax(0,1fr)_7rem_4.5rem_5.5rem] md:px-6"
					>
						<StatusIcon status={d.status} />
						<span class="flex min-w-0 items-center gap-2.5">
							<ProjectAvatar seed={p?.slug ?? String(d.project_id)} class="size-5" />
							<span class="truncate font-medium">{p?.name ?? `Project ${d.project_id}`}</span>
							<span class="shrink-0 font-mono text-[13px] text-muted-foreground">#{d.id}</span>
						</span>
						<span
							class="col-start-2 row-start-2 flex min-w-0 items-center gap-2 md:col-start-auto md:row-start-auto"
						>
							<span class="truncate">{d.commit_message.split('\n')[0] || 'No commit message'}</span>
							<span
								class="hidden shrink-0 items-center gap-1 font-mono text-xs text-muted-foreground lg:inline-flex"
							>
								<GitBranchIcon class="size-3.5" />{shortRef(d.ref)}
								{#if d.commit_sha}<span aria-hidden="true">·</span>{d.commit_sha.slice(0, 7)}{/if}
							</span>
						</span>
						<StatusDot status={d.status} class="hidden text-[13px] md:inline-flex" />
						<span class="hidden text-right font-mono text-[13px] tabular-nums md:block">
							{ms === null ? '' : formatDuration(ms)}
						</span>
						<time
							class="col-start-3 row-start-1 text-right text-xs text-muted-foreground md:col-start-auto md:row-start-auto md:text-[13px]"
							datetime={d.queued_at}
							title={formatDateTime(d.queued_at)}>{shortAgo(d.queued_at, now)}</time
						>
					</a>
				</li>
			{/each}
		</ul>
	{/if}
</section>

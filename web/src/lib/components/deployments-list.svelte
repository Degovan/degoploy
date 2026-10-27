<script lang="ts">
	import { createQuery } from '@tanstack/svelte-query';
	import { resolve } from '$app/paths';
	import { deploymentKeys, deploymentsApi, shortRef } from '#lib/api/deployments.js';
	import DeployStatusBadge from '#lib/components/deploy-status-badge.svelte';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import * as Card from '#lib/components/ui/card/index.js';
	import { deploymentDuration, formatDateTime, formatDuration, timeAgo } from '#lib/time.js';
	import RocketIcon from '@lucide/svelte/icons/rocket';
	import type { Snippet } from 'svelte';

	let { projectId, action }: { projectId: number; action?: Snippet } = $props();

	const deployments = createQuery(() => ({
		queryKey: deploymentKeys.list(projectId),
		queryFn: () => deploymentsApi.list(projectId)
	}));

	// Re-render running durations every second.
	let now = $state(Date.now());
	$effect(() => {
		const t = setInterval(() => (now = Date.now()), 1000);
		return () => clearInterval(t);
	});

	const trigger = (d: { trigger: string; triggered_by: string; rule_name: string }) =>
		d.trigger === 'webhook'
			? `webhook${d.rule_name ? ` · ${d.rule_name}` : ''}`
			: `${d.trigger} by ${d.triggered_by || 'unknown'}`;
</script>

<Card.Root>
	<Card.Header>
		<Card.Title>Deployments</Card.Title>
		<Card.Description>Live status of the latest runs.</Card.Description>
		{#if action}<Card.Action>{@render action()}</Card.Action>{/if}
	</Card.Header>
	<Card.Content class="px-0">
		{#if deployments.isPending}
			<div class="px-6"><Skeleton class="h-24 w-full" /></div>
		{:else if deployments.isError}
			<p class="px-6 text-sm text-destructive">{deployments.error.message}</p>
		{:else if deployments.data.length === 0}
			<div
				class="mx-6 rounded-md border border-dashed p-6 text-center text-sm text-muted-foreground"
			>
				<RocketIcon class="mx-auto mb-2 size-5" />
				No deployments yet. Push to a branch matched by a trigger rule, or use Deploy now.
			</div>
		{:else}
			<ul class="flex flex-col">
				{#each deployments.data as d (d.id)}
					{@const ms = deploymentDuration(d, now)}
					<li>
						<a
							href={resolve('/(app)/projects/[id]/deployments/[did]', {
								id: String(projectId),
								did: String(d.id)
							})}
							class="flex items-start gap-3 px-6 py-3 transition-colors hover:bg-muted/50"
						>
							<DeployStatusBadge status={d.status} class="mt-0.5 w-24 justify-center" />
							<div class="flex min-w-0 flex-1 flex-col gap-0.5">
								<div class="flex flex-wrap items-center gap-x-2 text-sm">
									<span class="font-medium">#{d.id}</span>
									<span class="font-mono text-xs">{shortRef(d.ref)}</span>
									{#if d.commit_sha}
										<code class="text-xs text-muted-foreground">{d.commit_sha.slice(0, 7)}</code>
									{/if}
									<span class="truncate text-muted-foreground"
										>{d.commit_message.split('\n')[0]}</span
									>
								</div>
								<span class="text-xs text-muted-foreground">
									{trigger(d)}{d.author ? ` · ${d.author}` : ''}
								</span>
							</div>
							<div class="flex shrink-0 flex-col items-end gap-0.5 text-xs text-muted-foreground">
								<time datetime={d.queued_at} title={formatDateTime(d.queued_at)}
									>{timeAgo(d.queued_at, now)}</time
								>
								{#if ms !== null}<span class="tabular-nums">{formatDuration(ms)}</span>{/if}
							</div>
						</a>
					</li>
				{/each}
			</ul>
		{/if}
	</Card.Content>
</Card.Root>

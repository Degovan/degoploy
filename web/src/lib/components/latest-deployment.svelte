<script lang="ts">
	import { useQueryClient } from '@tanstack/svelte-query';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { toast } from 'svelte-sonner';
	import { errorMessage } from '#lib/api/client.js';
	import {
		cacheDeployment,
		deploymentKeys,
		deploymentsApi,
		isActive,
		shortRef
	} from '#lib/api/deployments.js';
	import type { Deployment, Project } from '#lib/api/types.gen.js';
	import Pipeline from '#lib/components/pipeline.svelte';
	import SectionHeading from '#lib/components/section-heading.svelte';
	import StatusDot from '#lib/components/status-dot.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { parseRepo } from '#lib/repo.js';
	import { deploymentStages, runMs } from '#lib/stages.js';
	import { session } from '#lib/stores/session.svelte.js';
	import { formatDateTime, formatDuration, formatStage, shortAgo } from '#lib/time.js';
	import { cn } from '#lib/utils.js';
	import ArrowRightIcon from '@lucide/svelte/icons/arrow-right';
	import CircleXIcon from '@lucide/svelte/icons/circle-x';
	import GitBranchIcon from '@lucide/svelte/icons/git-branch';
	import GitCommitIcon from '@lucide/svelte/icons/git-commit-horizontal';
	import LoaderIcon from '@lucide/svelte/icons/loader-circle';
	import RotateCcwIcon from '@lucide/svelte/icons/rotate-ccw';
	import ScrollTextIcon from '@lucide/svelte/icons/scroll-text';

	let { project: p, deployment: d }: { project: Project; deployment: Deployment } = $props();
	const queryClient = useQueryClient();

	let now = $state(Date.now());
	$effect(() => {
		if (!isActive(d)) return;
		const t = setInterval(() => (now = Date.now()), 200);
		return () => clearInterval(t);
	});

	const stages = $derived(deploymentStages(d, p, isActive(d) ? now : Date.now()));
	const run = $derived(runMs(d, now));
	const waited = $derived(stages[0]!.ms);
	const logHref = $derived(
		resolve('/(app)/projects/[id]/deployments/[did]', { id: String(p.id), did: String(d.id) })
	);
	const repo = $derived(parseRepo(p.repo_url));
	const commitHref = $derived(repo && d.commit_sha ? `${repo.web}/commit/${d.commit_sha}` : null);

	const triggerText = $derived(
		d.trigger === 'webhook'
			? `Webhook${d.rule_name ? ` · ${d.rule_name}` : ''}`
			: d.trigger === 'manual'
				? 'Manual · Deploy now'
				: d.trigger === 'rollback'
					? `Rollback · release #${d.release_id ?? '?'}`
					: 'Redeploy'
	);
	const createdBy = $derived(d.triggered_by || d.author);

	let redeploying = $state(false);
	async function redeploy() {
		redeploying = true;
		try {
			const nd = await deploymentsApi.redeploy(d.id);
			cacheDeployment(queryClient, nd);
			await queryClient.invalidateQueries({ queryKey: deploymentKeys.list(p.id) });
			toast.success(`Deployment #${nd.id} queued`);
			await goto(
				resolve('/(app)/projects/[id]/deployments/[did]', {
					id: String(p.id),
					did: String(nd.id)
				})
			);
		} catch (err) {
			toast.error(errorMessage(err));
		} finally {
			redeploying = false;
		}
	}
</script>

{#snippet field(label: string)}
	<dt class="text-xs text-muted-foreground">{label}</dt>
{/snippet}

<section class="flex flex-col gap-4" aria-labelledby="latest-deployment">
	<SectionHeading title="Latest Deployment" id="latest-deployment">
		{#snippet actions()}
			<Button variant="outline" size="sm" href={logHref}>
				<ScrollTextIcon />
				Build logs
			</Button>
			{#if session.can('project.deploy') && d.ref && !isActive(d)}
				<Button variant="outline" size="sm" onclick={redeploy} disabled={redeploying}>
					{#if redeploying}<LoaderIcon class="animate-spin" />{:else}<RotateCcwIcon />{/if}
					Redeploy
				</Button>
			{/if}
		{/snippet}
	</SectionHeading>

	<div class="overflow-hidden rounded-xl border bg-card" data-testid="latest-deployment">
		<div class="grid lg:grid-cols-[minmax(0,1fr)_minmax(0,1.15fr)]">
			<div class="flex flex-col gap-6 border-b bg-background/60 p-6 lg:border-r lg:border-b-0">
				<div class="flex items-center justify-between text-sm">
					<span class="text-muted-foreground">Pipeline</span>
					{#if run !== null}
						<span class="text-xs text-muted-foreground">
							{isActive(d) ? 'Running for' : 'Took'}
							<span class="font-mono">{formatDuration(run)}</span>
						</span>
					{/if}
				</div>
				<Pipeline {stages} finished={!isActive(d)} />
			</div>

			<dl class="grid grid-cols-2 gap-x-6 gap-y-5 p-6 text-sm">
				<div class="flex min-w-0 flex-col gap-1">
					{@render field('Deployment')}
					<dd class="truncate">
						<a href={logHref} class="hover:underline">
							<span class="text-muted-foreground">#{d.id}</span>
							{d.commit_message.split('\n')[0] || shortRef(d.ref) || 'Manual deploy'}
						</a>
					</dd>
				</div>
				<div class="flex flex-col gap-1">
					{@render field('Status')}
					<dd class="flex items-center gap-2">
						<StatusDot status={d.status} />
						{#if d.exit_code !== null && d.exit_code !== 0}
							<span class="font-mono text-xs text-muted-foreground">exit {d.exit_code}</span>
						{/if}
					</dd>
				</div>
				<div class="flex min-w-0 flex-col gap-1">
					{@render field('Created')}
					<dd class="truncate" title={formatDateTime(d.queued_at)}>
						{shortAgo(d.queued_at, now)}{createdBy ? ` by ${createdBy}` : ''}
					</dd>
				</div>
				<div class="flex flex-col gap-1">
					{@render field('Duration')}
					<dd class="font-mono text-sm">
						{run === null ? '—' : formatDuration(run)}
						{#if waited !== null && d.started_at}
							<span class="text-muted-foreground"> · queued {formatStage(waited)}</span>
						{/if}
					</dd>
				</div>
				<div class="flex min-w-0 flex-col gap-1">
					{@render field('Source')}
					<dd class="flex min-w-0 flex-col gap-1 font-mono text-xs">
						<span class="flex min-w-0 items-center gap-1.5">
							<GitBranchIcon class="size-3.5 shrink-0 text-muted-foreground" />
							<span class="truncate">{shortRef(d.ref) || '—'}</span>
						</span>
						{#if d.commit_sha}
							<span class="flex items-center gap-1.5">
								<GitCommitIcon class="size-3.5 shrink-0 text-muted-foreground" />
								{#if commitHref}
									<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external link -->
									<a href={commitHref} target="_blank" rel="noreferrer" class="hover:underline"
										>{d.commit_sha.slice(0, 7)}</a
									>
								{:else}
									{d.commit_sha.slice(0, 7)}
								{/if}
							</span>
						{/if}
					</dd>
				</div>
				<div class="flex min-w-0 flex-col gap-1">
					{@render field('Trigger')}
					<dd class="truncate">{triggerText}</dd>
				</div>
			</dl>
		</div>

		{#if d.error && !isActive(d)}
			{@const failed = d.status === 'failed' || d.status === 'timeout'}
			<div
				class={cn(
					'flex items-center gap-3 border-t px-6 py-3 text-sm',
					failed ? 'bg-red-500/10 text-red-600 dark:text-red-400' : 'text-muted-foreground'
				)}
				data-testid="deployment-error"
			>
				<CircleXIcon class="size-4 shrink-0" />
				<code class="min-w-0 flex-1 truncate font-mono text-xs" title={d.error}>{d.error}</code>
				<a
					href={logHref}
					class="flex shrink-0 items-center gap-1 text-sm text-foreground hover:underline"
				>
					View logs <ArrowRightIcon class="size-3.5" />
				</a>
			</div>
		{/if}
	</div>
</section>

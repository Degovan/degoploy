<script lang="ts">
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { toast } from 'svelte-sonner';
	import { ApiError, errorMessage } from '#lib/api/client.js';
	import {
		cacheDeployment,
		deploymentKeys,
		deploymentsApi,
		isActive,
		shortRef
	} from '#lib/api/deployments.js';
	import { projectKeys, projectsApi } from '#lib/api/projects.js';
	import type { Deployment, Project } from '#lib/api/types.gen.js';
	import type { Component } from 'svelte';
	import ConfirmButton from '#lib/components/confirm-button.svelte';
	import DeploymentSteps from '#lib/components/deployment-steps.svelte';
	import Pipeline from '#lib/components/pipeline.svelte';
	import StatusIcon from '#lib/components/status-icon.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import * as DropdownMenu from '#lib/components/ui/dropdown-menu/index.js';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import * as Alert from '#lib/components/ui/alert/index.js';
	import { providerLabel, providerLinks } from '#lib/repo.js';
	import { deploymentStages, type Stage } from '#lib/stages.js';
	import { session } from '#lib/stores/session.svelte.js';
	import {
		deploymentDuration,
		formatDateTime,
		formatDuration,
		formatStage,
		timeAgo
	} from '#lib/time.js';
	import { cn } from '#lib/utils.js';
	import ArrowLeftIcon from '@lucide/svelte/icons/arrow-left';
	import CalendarIcon from '@lucide/svelte/icons/calendar';
	import ChevronLeftIcon from '@lucide/svelte/icons/chevron-left';
	import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import DownloadIcon from '@lucide/svelte/icons/download';
	import EllipsisIcon from '@lucide/svelte/icons/ellipsis';
	import ExternalLinkIcon from '@lucide/svelte/icons/external-link';
	import GitBranchIcon from '@lucide/svelte/icons/git-branch';
	import HourglassIcon from '@lucide/svelte/icons/hourglass';
	import RotateCcwIcon from '@lucide/svelte/icons/rotate-ccw';
	import SquareIcon from '@lucide/svelte/icons/square';
	import TimerIcon from '@lucide/svelte/icons/timer';
	import UserIcon from '@lucide/svelte/icons/user';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	const queryClient = useQueryClient();

	// Kept fresh by realtime deployment.updated events (see the (app) layout).
	const deployment = createQuery(() => ({
		queryKey: deploymentKeys.detail(data.did),
		queryFn: () => deploymentsApi.get(data.did)
	}));
	// Shared with the project layout's query.
	const project = createQuery(() => ({
		queryKey: projectKeys.detail(data.id),
		queryFn: () => projectsApi.get(data.id)
	}));
	const neighbors = createQuery(() => ({
		queryKey: deploymentKeys.neighbors(data.id, data.did),
		queryFn: () => deploymentsApi.neighbors(data.did)
	}));

	let now = $state(Date.now());
	$effect(() => {
		const t = setInterval(() => (now = Date.now()), 1000);
		return () => clearInterval(t);
	});

	const canDeploy = $derived(session.can('project.deploy'));
	const backHref = $derived(resolve('/(app)/projects/[id]', { id: String(data.id) }));
	const deploymentHref = (did: number) =>
		resolve('/(app)/projects/[id]/deployments/[did]', { id: String(data.id), did: String(did) });

	async function cancel() {
		try {
			cacheDeployment(queryClient, await deploymentsApi.cancel(data.did));
			toast.success('Cancel requested');
		} catch (err) {
			toast.error(errorMessage(err));
		}
	}

	async function redeploy() {
		try {
			const d = await deploymentsApi.redeploy(data.did);
			cacheDeployment(queryClient, d);
			toast.success(`Deployment #${d.id} queued`);
			await goto(deploymentHref(d.id));
		} catch (err) {
			toast.error(errorMessage(err));
		}
	}

	function triggerText(d: Deployment) {
		if (d.trigger === 'webhook') return d.author ? `Push by ${d.author}` : 'Webhook push';
		return `${d.trigger === 'manual' ? 'Manual' : 'Redeploy'} by ${d.triggered_by || 'unknown'}`;
	}

	type Meta = { icon: Component; text: string; title?: string };
	function meta(d: Deployment, stages: Stage[], ms: number | null): Meta[] {
		const sha = d.commit_sha.slice(0, 7);
		const out: Meta[] = [
			d.started_at
				? {
						icon: CalendarIcon,
						text: `Started ${timeAgo(d.started_at, now)}`,
						title: formatDateTime(d.started_at)
					}
				: {
						icon: CalendarIcon,
						text: `Queued ${timeAgo(d.queued_at, now)}`,
						title: formatDateTime(d.queued_at)
					}
		];
		if (d.started_at && stages[0].ms !== null)
			out.push({ icon: HourglassIcon, text: `Queued ${formatStage(stages[0].ms)}` });
		if (ms !== null)
			out.push({
				icon: TimerIcon,
				text: `${isActive(d) ? 'Running for' : 'Took'} ${formatDuration(ms)}`
			});
		out.push({ icon: UserIcon, text: triggerText(d) });
		if (d.ref)
			out.push({ icon: GitBranchIcon, text: `${shortRef(d.ref)}${sha ? ` @ ${sha}` : ''}` });
		return out;
	}

	/** What the deploy script saw in its environment (rule variables aside). */
	function scriptEnv(d: Deployment, p: Project): [string, string, string?][] {
		const ref = shortRef(d.ref);
		const refVar = d.ref.startsWith('refs/heads/')
			? 'DEPLOY_BRANCH'
			: d.ref.startsWith('refs/tags/')
				? 'DEPLOY_TAG'
				: 'DEPLOY_REF';
		return [
			['DEPLOY_ID', String(d.id)],
			['DEPLOY_PROJECT', p.slug],
			['DEPLOY_TRIGGER', d.trigger],
			[refVar, refVar === 'DEPLOY_REF' ? d.ref : ref],
			['DEPLOY_COMMIT', d.commit_sha ? `${d.commit_sha.slice(0, 7)}…` : '', d.commit_sha],
			['DEPLOY_AUTHOR', d.author]
		];
	}
</script>

<svelte:head>
	{#if deployment.data && project.data}
		<title>#{deployment.data.id} · {project.data.name} · degoploy</title>
	{/if}
</svelte:head>

{#if deployment.isPending || project.isPending}
	<div class="container-page flex flex-col gap-4 py-10">
		<Skeleton class="h-10 w-80" />
		<Skeleton class="h-40 w-full rounded-xl" />
	</div>
{:else if deployment.isError}
	<div class="container-page py-10">
		<Alert.Root variant="destructive">
			<CircleAlertIcon />
			<Alert.Title>
				{deployment.error instanceof ApiError && deployment.error.status === 404
					? 'Deployment not found'
					: deployment.error.message}
			</Alert.Title>
		</Alert.Root>
	</div>
{:else if project.isSuccess}
	{@const d = deployment.data}
	{@const p = project.data}
	{@const stages = deploymentStages(d, p, now)}
	{@const ms = deploymentDuration(d, now)}
	{@const links = providerLinks(p.provider, p.repo_url)}
	{@const sha = d.commit_sha.slice(0, 7)}
	{@const nav = neighbors.data}

	<section class="border-b">
		<div
			class="container-page flex flex-wrap items-center justify-between gap-x-8 gap-y-5 py-8 md:flex-nowrap"
		>
			<div class="flex min-w-0 flex-col gap-3.5">
				<a
					href={backHref}
					class="flex w-fit items-center gap-1.5 text-[13px] text-muted-foreground hover:text-foreground"
				>
					<ArrowLeftIcon class="size-3.5" />
					Deployments
				</a>
				<div class="flex min-w-0 items-center gap-3.5">
					<StatusIcon status={d.status} class="size-9 border border-current/50 [&_svg]:size-5" />
					<h1
						class="flex min-w-0 items-baseline gap-3 text-3xl font-semibold tracking-[-1px] md:text-[32px]"
					>
						<span class="shrink-0">#{d.id}</span>
						<span class="truncate text-muted-foreground" title={d.commit_message}>
							{d.commit_message.split('\n')[0] || 'No commit message'}
						</span>
					</h1>
				</div>
				<ul class="flex flex-wrap items-center gap-x-5 gap-y-1.5 text-[13px] text-muted-foreground">
					{#each meta(d, stages, ms) as m (m.icon)}
						<li class="flex items-center gap-1.5" title={m.title}>
							<m.icon class="size-3.5 text-muted-foreground/80" />{m.text}
						</li>
					{/each}
				</ul>
			</div>

			<div class="flex shrink-0 items-center gap-2">
				<nav
					class="flex h-9 items-stretch overflow-hidden rounded-md border"
					aria-label="Deployments"
				>
					{#each [['prev', nav?.prev ?? null], ['next', nav?.next ?? null]] as const as [dir, id] (dir)}
						{@const cls = cn(
							'flex items-center gap-1 px-2.5 font-mono text-[13px] text-muted-foreground',
							dir === 'prev' && 'border-r'
						)}
						{#if id !== null}
							<a
								href={deploymentHref(id)}
								class={cn(cls, 'transition-colors hover:bg-accent hover:text-foreground')}
								aria-label={dir === 'prev'
									? `Previous deployment #${id}`
									: `Next deployment #${id}`}
							>
								{#if dir === 'prev'}<ChevronLeftIcon
										class="size-[15px]"
									/>#{id}{:else}#{id}<ChevronRightIcon class="size-[15px]" />{/if}
							</a>
						{:else}
							<span class={cn(cls, 'opacity-40')} aria-hidden="true">
								{#if dir === 'prev'}<ChevronLeftIcon class="size-[15px]" />{:else}<ChevronRightIcon
										class="size-[15px]"
									/>{/if}
							</span>
						{/if}
					{/each}
				</nav>

				<DropdownMenu.Root>
					<DropdownMenu.Trigger>
						{#snippet child({ props })}
							<Button variant="outline" size="icon" aria-label="More actions" {...props}>
								<EllipsisIcon />
							</Button>
						{/snippet}
					</DropdownMenu.Trigger>
					<DropdownMenu.Content align="end" class="w-52">
						{#if links && d.commit_sha}
							<DropdownMenu.Item>
								{#snippet child({ props })}
									<!-- eslint-disable svelte/no-navigation-without-resolve -- external link -->
									<a
										href={`${links.repo}/commit/${d.commit_sha}`}
										target="_blank"
										rel="noreferrer"
										{...props}
									>
										View commit
										<ExternalLinkIcon class="ml-auto" />
									</a>
									<!-- eslint-enable svelte/no-navigation-without-resolve -->
								{/snippet}
							</DropdownMenu.Item>
						{/if}
						{#if links}
							<DropdownMenu.Item>
								{#snippet child({ props })}
									<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external link -->
									<a href={links.repo} target="_blank" rel="noreferrer" {...props}>
										Open on {providerLabel[p.provider]}
										<ExternalLinkIcon class="ml-auto" />
									</a>
								{/snippet}
							</DropdownMenu.Item>
						{/if}
						{#if d.started_at}
							<DropdownMenu.Item>
								{#snippet child({ props })}
									<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- API download -->
									<a href={deploymentsApi.logUrl(d.id)} download {...props}>
										Download log
										<DownloadIcon class="ml-auto" />
									</a>
								{/snippet}
							</DropdownMenu.Item>
						{/if}
					</DropdownMenu.Content>
				</DropdownMenu.Root>

				{#if canDeploy}
					{#if isActive(d)}
						<ConfirmButton
							title={`Cancel deployment #${d.id}?`}
							description="The deploy script receives SIGTERM, then SIGKILL after 10 seconds. Work it already did is not rolled back."
							confirmLabel="Cancel deployment"
							destructive
							onconfirm={cancel}
						>
							<SquareIcon />
							Cancel
						</ConfirmButton>
					{:else if d.ref}
						<Button class="h-9 px-3.5 text-[13px]" onclick={redeploy}>
							<RotateCcwIcon />
							Redeploy
						</Button>
					{/if}
				{/if}
			</div>
		</div>
	</section>

	<div class="container-page flex flex-col gap-6 pt-8 pb-18">
		<section
			class="flex flex-col gap-5 rounded-xl border bg-card px-6 pt-5 pb-8"
			aria-label="Graph"
		>
			<span class="text-[13px] text-muted-foreground">Graph</span>
			<div class="overflow-x-auto">
				<Pipeline {stages} finished={!isActive(d)} class="mx-auto w-full max-w-3xl min-w-[30rem]" />
			</div>
		</section>

		{#if d.status === 'skipped' || (d.status === 'canceled' && !d.started_at)}
			<p class="rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground">
				{d.status === 'skipped'
					? 'Skipped: a newer deployment for this project was queued before this one started.'
					: 'Canceled before it started.'}
			</p>
		{/if}

		<div class="grid items-start gap-6 lg:grid-cols-[20rem_minmax(0,1fr)]">
			<div class="order-2 flex flex-col gap-6 lg:order-1">
				<section
					class="flex flex-col gap-3.5 rounded-xl border bg-card px-5 py-[18px]"
					aria-labelledby="run-details"
				>
					<h2 id="run-details" class="text-sm font-semibold">Run details</h2>
					<dl class="grid grid-cols-[5.5rem_minmax(0,1fr)] gap-x-3 gap-y-3.5 text-[13px]">
						<dt class="text-muted-foreground">Trigger</dt>
						<dd class="truncate">
							{d.trigger === 'webhook'
								? 'Webhook · push'
								: `${d.trigger === 'manual' ? 'Manual' : 'Redeploy'} · ${d.triggered_by || 'unknown'}`}
						</dd>
						<dt class="text-muted-foreground">Rule</dt>
						<dd class="truncate">{d.rule_name || '—'}</dd>
						<dt class="text-muted-foreground">Commit</dt>
						<dd class="truncate font-mono">
							{#if sha && links}
								<!-- eslint-disable svelte/no-navigation-without-resolve -- external link -->
								<a
									href={`${links.repo}/commit/${d.commit_sha}`}
									target="_blank"
									rel="noreferrer"
									class="hover:underline"
									title={d.commit_sha}>{sha}</a
								>
								<!-- eslint-enable svelte/no-navigation-without-resolve -->
							{:else}
								{sha || '—'}
							{/if}
						</dd>
						<dt class="text-muted-foreground">Author</dt>
						<dd class="truncate">{d.author || '—'}</dd>
						<dt class="text-muted-foreground">Script</dt>
						<dd class="truncate">
							{#if p.script_source === 'repo'}
								<span class="font-mono" title={p.script_path}>{p.script_path}</span>
							{:else if d.script_revision_id !== null}
								<a
									href={resolve('/(app)/projects/[id]/script', { id: String(p.id) })}
									class="hover:underline">Revision #{d.script_revision_id}</a
								>
							{:else}
								<span class="text-muted-foreground">Managed script</span>
							{/if}
						</dd>
						<dt class="text-muted-foreground">Working dir</dt>
						<dd class="truncate font-mono" title={p.work_dir}>{p.work_dir}</dd>
					</dl>
				</section>

				<section
					class="flex flex-col gap-3.5 rounded-xl border bg-card px-5 py-[18px]"
					aria-labelledby="environment"
				>
					<h2 id="environment" class="text-sm font-semibold">Environment</h2>
					<dl
						class="flex flex-col gap-2 overflow-hidden rounded-md border bg-background px-3.5 py-3 font-mono text-xs"
					>
						{#each scriptEnv(d, p) as [key, value, full] (key)}
							<div class="flex min-w-0" title={full ?? value}>
								<dt class="shrink-0 text-sky-600 dark:text-sky-400">{key}</dt>
								<span class="shrink-0 text-muted-foreground">=</span>
								<dd class="truncate">{value}</dd>
							</div>
						{/each}
					</dl>
				</section>
			</div>

			<div class="order-1 min-w-0 lg:order-2">
				{#key d.id}
					<DeploymentSteps deployment={d} project={p} {stages} {now} />
				{/key}
			</div>
		</div>
	</div>
{/if}

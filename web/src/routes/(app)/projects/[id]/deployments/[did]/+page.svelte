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
	import DeployStatusBadge from '#lib/components/deploy-status-badge.svelte';
	import LogViewer from '#lib/components/log-viewer.svelte';
	import ConfirmButton from '#lib/components/confirm-button.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Badge } from '#lib/components/ui/badge/index.js';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import * as Card from '#lib/components/ui/card/index.js';
	import * as Alert from '#lib/components/ui/alert/index.js';
	import { session } from '#lib/stores/session.svelte.js';
	import { deploymentDuration, formatDateTime, formatDuration, timeAgo } from '#lib/time.js';
	import ArrowLeftIcon from '@lucide/svelte/icons/arrow-left';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import DownloadIcon from '@lucide/svelte/icons/download';
	import RotateCcwIcon from '@lucide/svelte/icons/rotate-ccw';
	import SquareIcon from '@lucide/svelte/icons/square';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	const queryClient = useQueryClient();

	// Kept fresh by realtime deployment.updated events (see the (app) layout).
	const deployment = createQuery(() => ({
		queryKey: deploymentKeys.detail(data.did),
		queryFn: () => deploymentsApi.get(data.did)
	}));

	let now = $state(Date.now());
	$effect(() => {
		const t = setInterval(() => (now = Date.now()), 1000);
		return () => clearInterval(t);
	});

	const canDeploy = $derived(session.can('project.deploy'));
	const backHref = $derived(resolve('/(app)/projects/[id]', { id: String(data.id) }));

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
			await goto(
				resolve('/(app)/projects/[id]/deployments/[did]', {
					id: String(data.id),
					did: String(d.id)
				})
			);
		} catch (err) {
			toast.error(errorMessage(err));
		}
	}
</script>

{#if deployment.isPending}
	<Skeleton class="h-40 w-full rounded-xl" />
{:else if deployment.isError}
	<Alert.Root variant="destructive">
		<CircleAlertIcon />
		<Alert.Title>
			{deployment.error instanceof ApiError && deployment.error.status === 404
				? 'Deployment not found'
				: deployment.error.message}
		</Alert.Title>
	</Alert.Root>
{:else}
	{@const d = deployment.data}
	{@const ms = deploymentDuration(d, now)}
	<div class="flex min-w-0 flex-col gap-6">
		<div class="flex flex-wrap items-start justify-between gap-4">
			<div class="flex min-w-0 flex-col gap-2">
				<a
					href={backHref}
					class="flex w-fit items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
				>
					<ArrowLeftIcon class="size-4" />
					Overview
				</a>
				<div class="flex flex-wrap items-center gap-2">
					<h2 class="text-xl font-semibold tracking-tight">Deployment #{d.id}</h2>
					<DeployStatusBadge status={d.status} />
					{#if d.exit_code !== null && d.status === 'failed'}
						<Badge variant="outline" class="font-mono">exit {d.exit_code}</Badge>
					{/if}
				</div>
				{#if d.commit_message}
					<p class="max-w-3xl truncate text-sm">{d.commit_message.split('\n')[0]}</p>
				{/if}
			</div>
			{#if canDeploy}
				<div class="flex gap-2">
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
						<Button variant="outline" onclick={redeploy}>
							<RotateCcwIcon />
							Redeploy
						</Button>
					{/if}
				</div>
			{/if}
		</div>

		<Card.Root class="py-4">
			<Card.Content>
				<dl class="grid gap-x-8 gap-y-3 text-sm sm:grid-cols-2 lg:grid-cols-4">
					<div>
						<dt class="text-xs text-muted-foreground">Ref</dt>
						<dd class="font-mono text-xs">{shortRef(d.ref) || '–'}</dd>
					</div>
					<div>
						<dt class="text-xs text-muted-foreground">Commit</dt>
						<dd class="font-mono text-xs">
							{d.commit_sha ? d.commit_sha.slice(0, 12) : 'resolved at run time'}
						</dd>
					</div>
					<div>
						<dt class="text-xs text-muted-foreground">Triggered by</dt>
						<dd>
							{#if d.trigger === 'webhook'}
								Webhook{d.rule_name ? ` · rule “${d.rule_name}”` : ''}
							{:else}
								{d.trigger === 'manual' ? 'Manual' : 'Redeploy'} by {d.triggered_by || 'unknown'}
							{/if}
						</dd>
					</div>
					<div>
						<dt class="text-xs text-muted-foreground">Author</dt>
						<dd>{d.author || '–'}</dd>
					</div>
					<div>
						<dt class="text-xs text-muted-foreground">Queued</dt>
						<dd title={formatDateTime(d.queued_at)}>{timeAgo(d.queued_at, now)}</dd>
					</div>
					<div>
						<dt class="text-xs text-muted-foreground">Started</dt>
						<dd>{d.started_at ? formatDateTime(d.started_at) : '–'}</dd>
					</div>
					<div>
						<dt class="text-xs text-muted-foreground">Duration</dt>
						<dd class="tabular-nums">{ms !== null ? formatDuration(ms) : '–'}</dd>
					</div>
				</dl>
			</Card.Content>
		</Card.Root>

		<div class="flex flex-col gap-2">
			<div class="flex items-center justify-between">
				<h3 class="text-sm font-medium">Log</h3>
				{#if d.started_at}
					<!-- eslint-disable svelte/no-navigation-without-resolve -- API download -->
					<a
						href={deploymentsApi.logUrl(d.id)}
						class="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
						download
					>
						<DownloadIcon class="size-3.5" />
						Download
					</a>
					<!-- eslint-enable svelte/no-navigation-without-resolve -->
				{/if}
			</div>
			{#if d.status === 'skipped' || (d.status === 'canceled' && !d.started_at)}
				<p class="rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground">
					{d.status === 'skipped'
						? 'Skipped: a newer deployment for this project was queued before this one started.'
						: 'Canceled before it started.'}
				</p>
			{:else}
				{#key d.id}
					<LogViewer deploymentId={d.id} active={isActive(d)} />
				{/key}
			{/if}
		</div>
	</div>
{/if}

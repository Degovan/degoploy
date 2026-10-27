<script lang="ts" module>
	import type { WebhookEvent } from '#lib/api/types.gen.js';

	export type DeliveryKind = 'queued' | 'ping' | 'rejected' | 'skipped';

	/** What degoploy answered the delivery; "ignored" (unused event types) reads as skipped. */
	export function deliveryKind(e: WebhookEvent): DeliveryKind {
		switch (e.status) {
			case 'queued':
			case 'rejected':
				return e.status;
			case 'pong':
				return 'ping';
			default:
				return 'skipped';
		}
	}
</script>

<script lang="ts">
	import { createQuery } from '@tanstack/svelte-query';
	import { resolve } from '$app/paths';
	import { ruleKeys, rulesApi } from '#lib/api/rules.js';
	import { shortRef } from '#lib/api/deployments.js';
	import { realtime } from '#lib/realtime.svelte.js';
	import SectionHeading from '#lib/components/section-heading.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import { formatDateTime, shortAgo } from '#lib/time.js';
	import { cn } from '#lib/utils.js';
	import CircleCheckIcon from '@lucide/svelte/icons/circle-check';
	import CircleMinusIcon from '@lucide/svelte/icons/circle-minus';
	import CircleXIcon from '@lucide/svelte/icons/circle-x';
	import ExternalLinkIcon from '@lucide/svelte/icons/external-link';
	import RadioTowerIcon from '@lucide/svelte/icons/radio-tower';
	import WebhookIcon from '@lucide/svelte/icons/webhook';

	let { projectId, settingsUrl }: { projectId: number; settingsUrl?: string } = $props();

	// Realtime "webhook.received" events refresh this; poll only while disconnected.
	const events = createQuery(() => ({
		queryKey: ruleKeys.events(projectId),
		queryFn: () => rulesApi.events(projectId),
		refetchInterval: realtime.status === 'open' ? false : 30_000
	}));
</script>

<section class="flex flex-col gap-4" aria-labelledby="webhook-deliveries">
	<SectionHeading
		title="Webhook Deliveries"
		description="The last 50 deliveries and what degoploy did with them."
		id="webhook-deliveries"
	/>

	{#if events.isPending}
		<Skeleton class="h-32 w-full rounded-xl" />
	{:else if events.isError}
		<p class="text-sm text-destructive">{events.error.message}</p>
	{:else if events.data.length === 0}
		<div class="flex flex-col items-center gap-2 rounded-xl border bg-card px-6 py-14 text-center">
			<span class="mb-2 flex size-11 items-center justify-center rounded-lg border bg-background">
				<WebhookIcon class="size-5" />
			</span>
			<p class="font-semibold">No deliveries yet</p>
			<p class="max-w-sm text-sm text-muted-foreground">
				Add the webhook above, then push a commit. Every delivery shows up here with the rule it
				matched.
			</p>
			{#if settingsUrl}
				<Button
					variant="outline"
					size="sm"
					class="mt-3"
					href={settingsUrl}
					target="_blank"
					rel="noreferrer"
				>
					<ExternalLinkIcon />
					Open webhook settings
				</Button>
			{/if}
		</div>
	{:else}
		<ul class="divide-y overflow-hidden rounded-xl border bg-card">
			{#each events.data as e (e.id)}
				{@const k = deliveryKind(e)}
				<li class="flex gap-3 px-5 py-3.5">
					<span class="mt-0.5">
						{#if k === 'queued'}
							<CircleCheckIcon class="size-4 text-emerald-500" />
						{:else if k === 'ping'}
							<RadioTowerIcon class="size-4 text-sky-500" />
						{:else if k === 'rejected'}
							<CircleXIcon class="size-4 text-red-500" />
						{:else}
							<CircleMinusIcon class="size-4 text-muted-foreground" />
						{/if}
					</span>
					<div class="flex min-w-0 flex-1 flex-col gap-1">
						<div class="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm">
							<span class="font-medium">{e.event}</span>
							{#if e.ref}
								<span class="font-mono text-xs text-muted-foreground">{shortRef(e.ref)}</span>
							{/if}
							{#if e.commit_sha}
								<code class="text-xs text-muted-foreground">{e.commit_sha.slice(0, 7)}</code>
							{/if}
							{#if e.deployment_id}
								<a
									href={resolve('/(app)/projects/[id]/deployments/[did]', {
										id: String(projectId),
										did: String(e.deployment_id)
									})}
									class="text-xs underline-offset-4 hover:underline"
									>Deployment #{e.deployment_id}</a
								>
							{/if}
							<time
								class="ml-auto text-xs text-muted-foreground"
								datetime={e.received_at}
								title={formatDateTime(e.received_at)}>{shortAgo(e.received_at)}</time
							>
						</div>
						<p
							class={cn(
								'text-xs break-words',
								k === 'rejected' ? 'text-red-500' : 'text-muted-foreground'
							)}
						>
							{e.reason}
						</p>
					</div>
				</li>
			{/each}
		</ul>
	{/if}
</section>

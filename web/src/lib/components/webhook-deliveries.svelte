<script lang="ts">
	import { createQuery } from '@tanstack/svelte-query';
	import { ruleKeys, rulesApi } from '#lib/api/rules.js';
	import { realtime } from '#lib/realtime.svelte.js';
	import type { WebhookEvent } from '#lib/api/types.gen.js';
	import { Badge } from '#lib/components/ui/badge/index.js';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import * as Card from '#lib/components/ui/card/index.js';
	import { formatDateTime, timeAgo } from '#lib/time.js';
	import { cn } from '#lib/utils.js';
	import CircleCheckIcon from '@lucide/svelte/icons/circle-check';
	import CircleMinusIcon from '@lucide/svelte/icons/circle-minus';
	import CircleXIcon from '@lucide/svelte/icons/circle-x';
	import RadioTowerIcon from '@lucide/svelte/icons/radio-tower';
	import WebhookIcon from '@lucide/svelte/icons/webhook';

	let { projectId }: { projectId: number } = $props();

	// Realtime "webhook.received" events refresh this; poll only while disconnected.
	const events = createQuery(() => ({
		queryKey: ruleKeys.events(projectId),
		queryFn: () => rulesApi.events(projectId),
		refetchInterval: realtime.status === 'open' ? false : 30_000
	}));

	type Kind = 'queued' | 'ping' | 'rejected' | 'skipped';
	function kind(e: WebhookEvent): Kind {
		if (e.matched) return 'queued';
		if (e.event === 'ping') return 'ping';
		if (/signature|invalid payload/.test(e.reason)) return 'rejected';
		return 'skipped';
	}
	const shortRef = (ref: string) => ref.replace(/^refs\/(heads|tags)\//, '');
</script>

<Card.Root>
	<Card.Header>
		<Card.Title>Webhook deliveries</Card.Title>
		<Card.Description>The last 50 deliveries and what degoploy did with them.</Card.Description>
	</Card.Header>
	<Card.Content>
		{#if events.isPending}
			<Skeleton class="h-24 w-full" />
		{:else if events.isError}
			<p class="text-sm text-destructive">{events.error.message}</p>
		{:else if events.data.length === 0}
			<div class="rounded-md border border-dashed p-6 text-center text-sm text-muted-foreground">
				<WebhookIcon class="mx-auto mb-2 size-5" />
				Nothing received yet. Add the webhook on the repository, then use its “Test delivery” or push
				a commit.
			</div>
		{:else}
			<ul class="flex flex-col divide-y">
				{#each events.data as e (e.id)}
					{@const k = kind(e)}
					<li class="flex gap-3 py-3 first:pt-0 last:pb-0">
						<span class="mt-0.5">
							{#if k === 'queued'}
								<CircleCheckIcon class="size-4 text-emerald-600" />
							{:else if k === 'ping'}
								<RadioTowerIcon class="size-4 text-sky-600" />
							{:else if k === 'rejected'}
								<CircleXIcon class="size-4 text-destructive" />
							{:else}
								<CircleMinusIcon class="size-4 text-muted-foreground" />
							{/if}
						</span>
						<div class="flex min-w-0 flex-1 flex-col gap-1">
							<div class="flex flex-wrap items-center gap-2 text-sm">
								<span class="font-medium">{e.event}</span>
								{#if e.ref}
									<Badge variant="outline" class="font-mono">{shortRef(e.ref)}</Badge>
								{/if}
								{#if e.commit_sha}
									<code class="text-xs text-muted-foreground">{e.commit_sha.slice(0, 7)}</code>
								{/if}
								{#if e.deployment_id}
									<Badge variant="secondary">Deployment #{e.deployment_id}</Badge>
								{/if}
								<time
									class="ml-auto text-xs text-muted-foreground"
									datetime={e.received_at}
									title={formatDateTime(e.received_at)}>{timeAgo(e.received_at)}</time
								>
							</div>
							<p
								class={cn(
									'text-xs break-words',
									k === 'rejected' ? 'text-destructive' : 'text-muted-foreground'
								)}
							>
								{e.reason}
							</p>
						</div>
					</li>
				{/each}
			</ul>
		{/if}
	</Card.Content>
</Card.Root>

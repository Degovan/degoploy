<script lang="ts">
	import { createQuery } from '@tanstack/svelte-query';
	import { ruleKeys, rulesApi } from '#lib/api/rules.js';
	import type { Project, ProjectSecrets } from '#lib/api/types.gen.js';
	import ConfirmButton from '#lib/components/confirm-button.svelte';
	import CopyField from '#lib/components/copy-field.svelte';
	import { deliveryKind } from '#lib/components/webhook-deliveries.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import { realtime } from '#lib/realtime.svelte.js';
	import { providerLabel, providerLinks } from '#lib/repo.js';
	import { formatDateTime, shortAgo } from '#lib/time.js';
	import { cn } from '#lib/utils.js';
	import BracesIcon from '@lucide/svelte/icons/braces';
	import ExternalLinkIcon from '@lucide/svelte/icons/external-link';
	import GitCommitIcon from '@lucide/svelte/icons/git-commit-horizontal';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import ShieldCheckIcon from '@lucide/svelte/icons/shield-check';
	import WebhookIcon from '@lucide/svelte/icons/webhook';

	let {
		project: p,
		secrets,
		onregenerate
	}: {
		project: Project;
		secrets: ProjectSecrets | undefined;
		onregenerate: () => Promise<void>;
	} = $props();

	const links = $derived(providerLinks(p.provider, p.repo_url));
	const url = $derived(
		secrets ? secrets.webhook_url || window.location.origin + secrets.webhook_path : ''
	);

	const events = createQuery(() => ({
		queryKey: ruleKeys.events(p.id),
		queryFn: () => rulesApi.events(p.id),
		refetchInterval: realtime.status === 'open' ? false : 30_000
	}));
	const last = $derived(events.data?.[0]);
	const lastKind = $derived(last ? deliveryKind(last) : null);

	// Push events also carry tag pushes. SSL verification only applies to https URLs.
	const chips = $derived(
		[
			{ icon: BracesIcon, label: 'application/json' },
			{ icon: GitCommitIcon, label: 'Push events only' },
			{ icon: ShieldCheckIcon, label: 'SSL verification on', show: url.startsWith('https://') }
		].filter((c) => c.show !== false)
	);
</script>

<div class="overflow-hidden rounded-xl border bg-card" data-testid="webhook-card">
	<div class="flex flex-col gap-5 p-6">
		<div class="flex flex-col gap-1.5">
			<h3 class="flex items-center gap-2 font-semibold">
				<WebhookIcon class="size-4" />
				Webhook
			</h3>
			<p class="text-sm text-muted-foreground">
				Create a webhook on the repository with these values.
				{#if p.provider === 'github'}
					GitHub sends a ping as soon as it is saved.
				{:else}
					Use <span class="text-foreground">Test Delivery</span> in Gitea to check it.
				{/if}
			</p>
		</div>
		{#if secrets}
			<dl class="grid items-center gap-x-6 gap-y-3 text-sm sm:grid-cols-[7rem_minmax(0,1fr)]">
				<dt class="text-muted-foreground">Payload URL</dt>
				<dd class="flex min-w-0 flex-col gap-1">
					<CopyField value={url} label="webhook URL" />
					{#if !secrets.webhook_url}
						<span class="text-xs text-muted-foreground">
							Built from this browser's address. Set <code>base_url</code> in the config so it always
							uses your public URL.
						</span>
					{/if}
				</dd>
				<dt class="text-muted-foreground">Secret</dt>
				<dd class="min-w-0">
					<CopyField value={secrets.webhook_secret} secret label="webhook secret" />
				</dd>
				<dt class="text-muted-foreground">Settings</dt>
				<dd class="flex flex-wrap gap-2">
					{#each chips as { icon: Icon, label } (label)}
						<span class="inline-flex h-6 items-center gap-1.5 rounded-full bg-muted px-2.5 text-xs">
							<Icon class="size-3 text-muted-foreground" />
							{label}
						</span>
					{/each}
				</dd>
			</dl>
		{:else}
			<Skeleton class="h-32 w-full" />
		{/if}
	</div>
	<div class="flex flex-wrap items-center gap-x-4 gap-y-2 border-t bg-background/40 px-6 py-3">
		<span class="flex min-w-0 items-center gap-2 text-sm" data-testid="delivery-status">
			<span
				class={cn(
					'size-2 shrink-0 rounded-full',
					!last
						? 'animate-pulse bg-amber-400'
						: lastKind === 'rejected'
							? 'bg-red-500'
							: 'bg-emerald-400'
				)}
			></span>
			{#if !last}
				<span class="text-muted-foreground">Waiting for first delivery</span>
			{:else}
				<span class="truncate" title={formatDateTime(last.received_at)}>
					Last delivery {shortAgo(last.received_at)}
					<span class="text-muted-foreground">· {lastKind}</span>
				</span>
			{/if}
		</span>
		<div class="ml-auto flex items-center gap-2">
			<ConfirmButton
				size="sm"
				variant="ghost"
				title="Generate a new webhook secret?"
				description="Webhooks signed with the old secret are rejected until you paste the new one into the repository's webhook settings."
				confirmLabel="Generate new secret"
				onconfirm={onregenerate}
			>
				<RefreshCwIcon />
				Regenerate secret
			</ConfirmButton>
			{#if links}
				<Button variant="outline" size="sm" href={links.webhooks} target="_blank" rel="noreferrer">
					<ExternalLinkIcon />
					Open {providerLabel[p.provider]}
				</Button>
			{/if}
		</div>
	</div>
</div>

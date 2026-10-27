<script lang="ts">
	import { createQuery } from '@tanstack/svelte-query';
	import { ssoApi, ssoKeys } from '#lib/api/sso.js';
	import PageHeader from '#lib/components/page-header.svelte';
	import CopyField from '#lib/components/copy-field.svelte';
	import { Badge } from '#lib/components/ui/badge/index.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import * as Card from '#lib/components/ui/card/index.js';
	import * as Alert from '#lib/components/ui/alert/index.js';
	import { formatDateTime, timeAgo } from '#lib/time.js';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';

	const status = createQuery(() => ({ queryKey: ssoKeys.status, queryFn: ssoApi.status }));

	const roleOrder: Record<string, number> = { admin: 0, deployer: 1, viewer: 2 };
	const mapping = $derived(
		Object.entries(status.data?.role_mapping ?? {}).sort(
			([ga, ra], [gb, rb]) => (roleOrder[ra] ?? 9) - (roleOrder[rb] ?? 9) || ga.localeCompare(gb)
		)
	);

	const example = `oidc:
  enabled: true
  provider_name: Casdoor
  issuer: https://sso.example.com
  client_id: <Casdoor application client ID>
  client_secret: \${DEGOPLOY_OIDC_SECRET}
  redirect_url: ${typeof window === 'undefined' ? '' : window.location.origin}/auth/oidc/callback
  scopes: [openid, profile, email]
  groups_claim: groups
  role_mapping:
    "my-org/degoploy-admins": admin
    "my-org/devops": deployer
    "*": viewer
  sync_role_on_login: true`;
</script>

<PageHeader
	title="Single sign-on"
	description="OpenID Connect settings and the latest SSO sign-ins, to check group → role mapping."
>
	{#snippet actions()}
		<Button variant="outline" onclick={() => status.refetch()} disabled={status.isFetching}>
			<RefreshCwIcon class={status.isFetching ? 'animate-spin' : ''} />
			Refresh
		</Button>
	{/snippet}
</PageHeader>

{#if status.isPending}
	<Skeleton class="h-64 w-full rounded-xl" />
{:else if status.isError}
	<Alert.Root variant="destructive">
		<CircleAlertIcon />
		<Alert.Title>{status.error.message}</Alert.Title>
	</Alert.Root>
{:else if !status.data.enabled}
	<Card.Root>
		<Card.Header>
			<Card.Title>SSO is not configured</Card.Title>
			<Card.Description>
				Add an <code>oidc</code> section to <code>degoploy.yaml</code> and restart degoploy. In
				Casdoor, create an Application, add the redirect URL below, and use token format
				<code>JWT</code> so groups are included.
			</Card.Description>
		</Card.Header>
		<Card.Content>
			<pre
				class="overflow-x-auto rounded-md border bg-muted/40 p-4 font-mono text-xs">{example}</pre>
		</Card.Content>
	</Card.Root>
{:else}
	{@const s = status.data}
	<div class="grid gap-6 xl:grid-cols-[1fr_22rem]">
		<Card.Root class="h-fit min-w-0">
			<Card.Header>
				<Card.Title class="flex items-center gap-2">
					{s.provider_name}
					<Badge variant="outline">Enabled</Badge>
					{#if !s.local_login}<Badge variant="secondary">Password login off</Badge>{/if}
				</Card.Title>
				<Card.Description>Configured in degoploy.yaml; restart to apply changes.</Card.Description>
			</Card.Header>
			<Card.Content>
				<dl class="grid gap-4 text-sm sm:grid-cols-2">
					<div class="flex flex-col gap-1 sm:col-span-2">
						<dt class="text-xs text-muted-foreground">
							Redirect URL (register this in the provider)
						</dt>
						<dd><CopyField value={s.redirect_url} label="redirect URL" /></dd>
					</div>
					<div>
						<dt class="text-xs text-muted-foreground">Issuer</dt>
						<dd class="font-mono text-xs break-all">{s.issuer}</dd>
					</div>
					<div>
						<dt class="text-xs text-muted-foreground">Client ID</dt>
						<dd class="font-mono text-xs break-all">{s.client_id}</dd>
					</div>
					<div>
						<dt class="text-xs text-muted-foreground">Scopes</dt>
						<dd class="font-mono text-xs">{s.scopes.join(' ')}</dd>
					</div>
					<div>
						<dt class="text-xs text-muted-foreground">Groups claim</dt>
						<dd class="font-mono text-xs">{s.groups_claim}</dd>
					</div>
					<div class="sm:col-span-2">
						<dt class="text-xs text-muted-foreground">Role on each sign-in</dt>
						<dd>
							{s.sync_role_on_login
								? 'Updated from groups every time (manual role edits are overwritten)'
								: 'Set from groups when the account is created, then managed in Users'}
						</dd>
					</div>
				</dl>
			</Card.Content>
		</Card.Root>

		<Card.Root class="h-fit">
			<Card.Header>
				<Card.Title class="text-base">Group → role</Card.Title>
				<Card.Description>The highest role among a user's groups applies.</Card.Description>
			</Card.Header>
			<Card.Content class="flex flex-col gap-2">
				{#each mapping as [group, role] (group)}
					<div class="flex items-center justify-between gap-3 text-sm">
						<span class="font-mono text-xs break-all">
							{group === '*' ? '* (everyone else)' : group}
						</span>
						<Badge variant="outline" class="capitalize">{role}</Badge>
					</div>
				{/each}
				{#if !s.role_mapping['*']}
					<p class="pt-1 text-xs text-muted-foreground">
						Users in none of these groups are refused.
					</p>
				{/if}
			</Card.Content>
		</Card.Root>
	</div>

	<Card.Root>
		<Card.Header>
			<Card.Title>Recent SSO sign-ins</Card.Title>
			<Card.Description>
				The last 20 attempts since degoploy started, with the claims the provider sent. Use them to
				find the exact group names for the mapping.
			</Card.Description>
		</Card.Header>
		<Card.Content>
			{#if s.recent_logins.length === 0}
				<p class="text-sm text-muted-foreground">No SSO sign-ins yet.</p>
			{:else}
				<ul class="flex flex-col divide-y">
					{#each s.recent_logins as l, i (i)}
						<li class="flex flex-col gap-2 py-3 first:pt-0" data-testid="sso-login">
							<div class="flex flex-wrap items-center gap-2 text-sm">
								<Badge variant={l.outcome.startsWith('signed in') ? 'outline' : 'destructive'}>
									{l.outcome}
								</Badge>
								<span class="font-medium">{l.username || l.email || l.subject}</span>
								{#if l.role}<Badge variant="secondary" class="capitalize">{l.role}</Badge>{/if}
								<time class="ml-auto text-xs text-muted-foreground" title={formatDateTime(l.at)}
									>{timeAgo(l.at)}</time
								>
							</div>
							<div class="flex flex-wrap gap-1">
								{#each l.groups as g (g)}
									<Badge variant="outline" class="font-mono">{g}</Badge>
								{:else}
									<span class="text-xs text-muted-foreground">
										No groups in the <code>{s.groups_claim}</code> claim.
									</span>
								{/each}
							</div>
							<details class="text-xs">
								<summary class="cursor-pointer text-muted-foreground">Claims</summary>
								<pre
									class="mt-2 overflow-x-auto rounded-md border bg-muted/40 p-3 font-mono">{JSON.stringify(
										l.claims,
										null,
										2
									)}</pre>
							</details>
						</li>
					{/each}
				</ul>
			{/if}
		</Card.Content>
	</Card.Root>
{/if}

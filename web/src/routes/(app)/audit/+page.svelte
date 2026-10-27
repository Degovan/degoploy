<script lang="ts">
	import { createInfiniteQuery, createQuery } from '@tanstack/svelte-query';
	import { resolve } from '$app/paths';
	import { auditApi, auditKeys, userKeys, usersApi, type AuditQuery } from '#lib/api/users.js';
	import type { AuditEntry } from '#lib/api/types.gen.js';
	import PageHeader from '#lib/components/page-header.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Badge } from '#lib/components/ui/badge/index.js';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import * as Card from '#lib/components/ui/card/index.js';
	import * as Table from '#lib/components/ui/table/index.js';
	import * as Select from '#lib/components/ui/select/index.js';
	import * as Alert from '#lib/components/ui/alert/index.js';
	import { formatDateTime, timeAgo } from '#lib/time.js';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import LoaderIcon from '@lucide/svelte/icons/loader-circle';

	const categories = [
		{ value: '', label: 'All activity' },
		{ value: 'auth.', label: 'Sign-in' },
		{ value: 'project.', label: 'Projects' },
		{ value: 'rule.', label: 'Trigger rules' },
		{ value: 'deployment.', label: 'Deployments' },
		{ value: 'user.', label: 'Users' }
	];

	const actionLabels: Record<string, string> = {
		'auth.login': 'Signed in',
		'auth.login_failed': 'Failed sign-in',
		'auth.logout': 'Signed out',
		'auth.password_change': 'Changed own password',
		'project.create': 'Created project',
		'project.update': 'Updated project',
		'project.delete': 'Deleted project',
		'project.secrets_view': 'Viewed project secrets',
		'project.key_regenerate': 'Regenerated deploy key',
		'project.secret_regenerate': 'Regenerated webhook secret',
		'project.script_update': 'Edited deploy script',
		'rule.create': 'Added trigger rule',
		'rule.update': 'Updated trigger rule',
		'rule.delete': 'Deleted trigger rule',
		'rule.reorder': 'Reordered trigger rules',
		'deployment.manual': 'Started deployment',
		'deployment.cancel': 'Canceled deployment',
		'deployment.redeploy': 'Redeployed',
		'user.create': 'Added user',
		'user.update': 'Updated user',
		'user.delete': 'Deleted user',
		'user.password_reset': 'Reset user password'
	};

	const tone = (action: string) =>
		action === 'auth.login_failed' || action.endsWith('.delete')
			? 'destructive'
			: action.startsWith('auth.')
				? 'secondary'
				: 'outline';

	let category = $state('');
	let userFilter = $state('');

	const users = createQuery(() => ({ queryKey: userKeys.all, queryFn: usersApi.list }));
	const filter = $derived<AuditQuery>({
		action: category || undefined,
		user: userFilter ? Number(userFilter) : undefined
	});

	const audit = createInfiniteQuery(() => ({
		queryKey: auditKeys.list(filter),
		queryFn: ({ pageParam }) =>
			auditApi.list({ ...filter, before: pageParam || undefined, limit: 50 }),
		initialPageParam: 0,
		getNextPageParam: (last) => last.next_before ?? undefined
	}));

	const entries = $derived(audit.data?.pages.flatMap((p) => p.entries) ?? []);
	const categoryLabel = $derived(categories.find((c) => c.value === category)?.label);
	const userLabel = $derived(
		userFilter
			? (users.data?.find((u) => String(u.id) === userFilter)?.username ?? 'User')
			: 'Everyone'
	);

	function fmt(v: unknown): string {
		if (v === null || v === undefined || v === '') return '–';
		return typeof v === 'object' ? JSON.stringify(v) : String(v);
	}

	/** Human-readable lines for an entry's detail object. */
	function details(e: AuditEntry): string[] {
		const out: string[] = [];
		for (const [k, v] of Object.entries(e.detail)) {
			if (k === 'slug' || (k === 'username' && e.target_type === 'user')) continue;
			if (k === 'changes' && v && typeof v === 'object') {
				for (const [field, pair] of Object.entries(v as Record<string, [unknown, unknown]>)) {
					out.push(`${field}: ${fmt(pair[0])} → ${fmt(pair[1])}`);
				}
			} else {
				out.push(`${k}: ${fmt(v)}`);
			}
		}
		return out;
	}

	function target(e: AuditEntry): { label: string; href?: string } | null {
		if (e.target_type === 'project') {
			const slug = typeof e.detail.slug === 'string' ? e.detail.slug : `#${e.target_id}`;
			return e.action === 'project.delete'
				? { label: slug }
				: { label: slug, href: resolve('/(app)/projects/[id]', { id: e.target_id }) };
		}
		if (e.target_type === 'user') {
			return {
				label: '@' + (typeof e.detail.username === 'string' ? e.detail.username : e.target_id)
			};
		}
		if (e.target_type === 'deployment') return { label: `deployment #${e.target_id}` };
		return null;
	}
</script>

<PageHeader title="Audit log" description="Who did what, and when. Newest first." />

<div class="flex flex-wrap gap-2">
	<Select.Root type="single" bind:value={category}>
		<Select.Trigger class="w-44" aria-label="Activity type">{categoryLabel}</Select.Trigger>
		<Select.Content>
			{#each categories as c (c.value)}
				<Select.Item value={c.value} label={c.label}>{c.label}</Select.Item>
			{/each}
		</Select.Content>
	</Select.Root>
	<Select.Root type="single" bind:value={userFilter}>
		<Select.Trigger class="w-44" aria-label="User">{userLabel}</Select.Trigger>
		<Select.Content>
			<Select.Item value="" label="Everyone">Everyone</Select.Item>
			{#each users.data ?? [] as u (u.id)}
				<Select.Item value={String(u.id)} label={u.username}>{u.username}</Select.Item>
			{/each}
		</Select.Content>
	</Select.Root>
</div>

{#if audit.isPending}
	<Skeleton class="h-64 w-full rounded-xl" />
{:else if audit.isError}
	<Alert.Root variant="destructive">
		<CircleAlertIcon />
		<Alert.Title>{audit.error.message}</Alert.Title>
	</Alert.Root>
{:else}
	<Card.Root class="py-0">
		<Table.Root>
			<Table.Header>
				<Table.Row>
					<Table.Head class="w-40 pl-6">When</Table.Head>
					<Table.Head class="w-32">Who</Table.Head>
					<Table.Head>What</Table.Head>
					<Table.Head class="w-32 pr-6">IP</Table.Head>
				</Table.Row>
			</Table.Header>
			<Table.Body>
				{#each entries as e (e.id)}
					{@const t = target(e)}
					{@const lines = details(e)}
					<Table.Row data-testid="audit-row">
						<Table.Cell class="pl-6 align-top text-muted-foreground">
							<time datetime={e.created_at} title={formatDateTime(e.created_at)}
								>{timeAgo(e.created_at)}</time
							>
						</Table.Cell>
						<Table.Cell class="align-top"
							>{e.username || (e.user_id ? '(deleted)' : '–')}</Table.Cell
						>
						<Table.Cell class="align-top whitespace-normal">
							<div class="flex flex-wrap items-center gap-2">
								<Badge variant={tone(e.action)}>{actionLabels[e.action] ?? e.action}</Badge>
								{#if t?.href}
									<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- resolved in target() -->
									<a href={t.href} class="font-mono text-xs underline-offset-4 hover:underline"
										>{t.label}</a
									>
								{:else if t}
									<span class="font-mono text-xs">{t.label}</span>
								{/if}
							</div>
							{#if lines.length}
								<ul
									class="mt-1 flex flex-col gap-0.5 font-mono text-xs break-all text-muted-foreground"
								>
									{#each lines as line, i (i)}<li>{line}</li>{/each}
								</ul>
							{/if}
						</Table.Cell>
						<Table.Cell class="pr-6 align-top font-mono text-xs text-muted-foreground"
							>{e.ip || '–'}</Table.Cell
						>
					</Table.Row>
				{:else}
					<Table.Row>
						<Table.Cell colspan={4} class="py-10 text-center text-muted-foreground">
							No matching activity.
						</Table.Cell>
					</Table.Row>
				{/each}
			</Table.Body>
		</Table.Root>
	</Card.Root>

	{#if audit.hasNextPage}
		<div class="flex justify-center">
			<Button
				variant="outline"
				onclick={() => audit.fetchNextPage()}
				disabled={audit.isFetchingNextPage}
			>
				{#if audit.isFetchingNextPage}<LoaderIcon class="animate-spin" />{/if}
				Load more
			</Button>
		</div>
	{/if}
{/if}

<script lang="ts">
	import { createInfiniteQuery, createQuery } from '@tanstack/svelte-query';
	import { resolve } from '$app/paths';
	import { DropdownMenu as DropdownMenuPrimitive } from 'bits-ui';
	import { SvelteSet } from 'svelte/reactivity';
	import { auditApi, auditKeys, userKeys, usersApi, type AuditQuery } from '#lib/api/users.js';
	import type { AuditEntry } from '#lib/api/types.gen.js';
	import {
		auditCategories,
		categoryOf,
		describe,
		sameEvent,
		type AuditCategory
	} from '#lib/audit.js';
	import PageHeader from '#lib/components/page-header.svelte';
	import UserAvatar from '#lib/components/user-avatar.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import * as Alert from '#lib/components/ui/alert/index.js';
	import * as DropdownMenu from '#lib/components/ui/dropdown-menu/index.js';
	import * as Select from '#lib/components/ui/select/index.js';
	import { formatDateTime } from '#lib/time.js';
	import { cn } from '#lib/utils.js';
	import ArrowDownIcon from '@lucide/svelte/icons/arrow-down';
	import CheckIcon from '@lucide/svelte/icons/check';
	import ChevronDownIcon from '@lucide/svelte/icons/chevron-down';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import FolderGit2Icon from '@lucide/svelte/icons/folder-git-2';
	import GitBranchIcon from '@lucide/svelte/icons/git-branch';
	import HistoryIcon from '@lucide/svelte/icons/history';
	import KeyRoundIcon from '@lucide/svelte/icons/key-round';
	import LayersIcon from '@lucide/svelte/icons/layers';
	import LoaderIcon from '@lucide/svelte/icons/loader-circle';
	import LockIcon from '@lucide/svelte/icons/lock';
	import LogInIcon from '@lucide/svelte/icons/log-in';
	import LogOutIcon from '@lucide/svelte/icons/log-out';
	import MailIcon from '@lucide/svelte/icons/mail';
	import RocketIcon from '@lucide/svelte/icons/rocket';
	import SearchIcon from '@lucide/svelte/icons/search';
	import UserCogIcon from '@lucide/svelte/icons/user-cog';
	import UsersIcon from '@lucide/svelte/icons/users';

	const categoryIcons: Record<AuditCategory, typeof LayersIcon> = {
		'auth.': LogInIcon,
		'project.': FolderGit2Icon,
		'rule.': GitBranchIcon,
		'variable.': KeyRoundIcon,
		'deployment.': RocketIcon,
		'user.': UserCogIcon,
		'invite.': MailIcon
	};
	const iconFor = (action: string) =>
		action === 'auth.logout' ? LogOutIcon : categoryIcons[categoryOf(action) ?? 'auth.'];
	const categoryLabel = (action: string) =>
		auditCategories.find((c) => c.value === categoryOf(action))?.label ?? 'Other';

	// Filters. The search is debounced so typing does not fire a request per key.
	let search = $state('');
	let query = $state('');
	let category = $state<AuditCategory | ''>('');
	let sensitiveOnly = $state(false);
	let userFilter = $state('');
	let searchInput = $state<HTMLInputElement | null>(null);

	$effect(() => {
		const q = search.trim();
		const t = setTimeout(() => (query = q), 250);
		return () => clearTimeout(t);
	});

	const tz = -new Date().getTimezoneOffset();
	const filter = $derived<AuditQuery>({
		action: category || undefined,
		user: userFilter ? Number(userFilter) : undefined,
		q: query || undefined,
		sensitive: sensitiveOnly || undefined,
		tz
	});

	const users = createQuery(() => ({ queryKey: userKeys.all, queryFn: usersApi.list }));
	const names = $derived(
		new Map((users.data ?? []).map((u) => [u.username, u.display_name || u.username]))
	);

	// Start of the viewer's day, for the "today" counts.
	const midnight = new Date(new Date().setHours(0, 0, 0, 0)).toISOString(); // eslint-disable-line svelte/prefer-svelte-reactivity
	const summary = createQuery(() => ({
		queryKey: auditKeys.summary(midnight),
		queryFn: () => auditApi.summary(midnight)
	}));

	const audit = createInfiniteQuery(() => ({
		queryKey: auditKeys.list(filter),
		queryFn: ({ pageParam }) =>
			auditApi.list({ ...filter, before: pageParam || undefined, limit: 50 }),
		initialPageParam: 0,
		getNextPageParam: (last) => last.next_before ?? undefined
	}));

	const pages = $derived(audit.data?.pages ?? []);
	const entries = $derived(pages.flatMap((p) => p.entries));
	const total = $derived(pages[0]?.total ?? 0);
	const dayCounts = $derived(
		Object.assign({}, ...pages.map((p) => p.day_counts)) as Record<string, number>
	);

	// Days, each a list of events; an event folds in the repeats right after it.
	const dayKey = (d: Date) =>
		`${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
	type Event = { head: AuditEntry; repeats: AuditEntry[] };
	const days = $derived.by(() => {
		const out: { key: string; date: Date; events: Event[] }[] = [];
		for (const e of entries) {
			const date = new Date(e.created_at);
			const key = dayKey(date);
			let day = out.at(-1);
			if (!day || day.key !== key) {
				day = { key, date, events: [] };
				out.push(day);
			}
			const last = day.events.at(-1);
			if (last && sameEvent(last.head, e)) last.repeats.push(e);
			else day.events.push({ head: e, repeats: [] });
		}
		return out;
	});
	const expanded = new SvelteSet<number>();

	const today = dayKey(new Date());
	const yesterday = dayKey(new Date(Date.now() - 86_400_000));
	const longDate = new Intl.DateTimeFormat(undefined, {
		weekday: 'long',
		day: 'numeric',
		month: 'long'
	});
	const withYear = new Intl.DateTimeFormat(undefined, {
		weekday: 'long',
		day: 'numeric',
		month: 'long',
		year: 'numeric'
	});
	const clock = new Intl.DateTimeFormat(undefined, {
		hour: '2-digit',
		minute: '2-digit',
		hour12: false
	});
	const dayTitle = (key: string) =>
		key === today ? 'Today' : key === yesterday ? 'Yesterday' : '';
	const dayDate = (date: Date) =>
		(date.getFullYear() === new Date().getFullYear() ? longDate : withYear).format(date);

	const filtered = $derived(!!(category || sensitiveOnly || userFilter || query));
	const categoryName = $derived(
		auditCategories.find((c) => c.value === category)?.label ?? 'All activity'
	);
	const CategoryIcon = $derived(category ? categoryIcons[category] : LayersIcon);
	const actor = $derived(users.data?.find((u) => String(u.id) === userFilter));

	function onKeydown(e: KeyboardEvent) {
		if (e.key !== '/' || e.metaKey || e.ctrlKey || e.altKey) return;
		const el = e.target as HTMLElement;
		if (
			el.closest(
				'input, textarea, select, [contenteditable="true"], [role="dialog"], [role="menu"]'
			)
		)
			return;
		e.preventDefault();
		searchInput?.focus();
	}

	function clearFilters() {
		search = query = '';
		category = '';
		sensitiveOnly = false;
		userFilter = '';
	}

	const trigger =
		'flex h-10 w-full items-center gap-2 rounded-md border border-input bg-card px-3 text-sm outline-none transition-[color,box-shadow] focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/40 data-[state=open]:border-ring data-[state=open]:ring-3 data-[state=open]:ring-ring/40 md:w-[200px]';
	const menuItem = 'h-[34px] gap-2.5 rounded-md px-2.5 text-sm [&_svg]:size-[15px]';
</script>

<svelte:window onkeydown={onKeydown} />

{#snippet sentence(e: AuditEntry)}
	{@const s = describe(e, names)}
	<span class="text-sm font-medium">{s.actor}</span>
	<span class="text-sm text-muted-foreground">{s.verb}</span>
	{#if s.target}
		{#if s.link}
			<a
				href={s.link.deployment
					? resolve('/(app)/projects/[id]/deployments/[did]', {
							id: s.link.project,
							did: s.link.deployment
						})
					: resolve('/(app)/projects/[id]', { id: s.link.project })}
				class={cn(
					'underline-offset-2 hover:underline',
					s.mono ? 'font-mono text-[13px]' : 'text-sm font-medium'
				)}>{s.target}</a
			>
		{:else}
			<span class={s.mono ? 'font-mono text-[13px]' : 'text-sm font-medium'}>{s.target}</span>
		{/if}
	{/if}
	{#if s.suffix}<span class="text-sm text-muted-foreground">{s.suffix}</span>{/if}
{/snippet}

{#snippet row(e: AuditEntry, repeats: AuditEntry[], nested = false)}
	{@const s = describe(e, names)}
	{@const Icon = iconFor(e.action)}
	{@const open = expanded.has(e.id)}
	<li
		class={cn(
			'grid grid-cols-[40px_38px_minmax(0,1fr)] items-center gap-x-3.5 border-b px-4 py-3 md:grid-cols-[40px_38px_minmax(0,1fr)_110px] md:px-5',
			nested && 'bg-background/60'
		)}
		data-testid="audit-row"
	>
		<time
			class="font-mono text-xs text-muted-foreground/80"
			datetime={e.created_at}
			title={formatDateTime(e.created_at)}>{clock.format(new Date(e.created_at))}</time
		>
		<span class="relative size-[38px]">
			{#if e.username}
				<UserAvatar user={{ username: e.username, display_name: names.get(e.username) ?? '' }} />
			{:else}
				<span
					class="flex size-8 items-center justify-center rounded-full border border-input bg-muted text-muted-foreground"
				>
					<UsersIcon class="size-3.5" />
				</span>
			{/if}
			<span
				class={cn(
					'absolute right-0 bottom-0 flex size-[18px] items-center justify-center rounded-full border-2 border-card',
					e.sensitive ? 'bg-[#3A2C10] text-amber-400' : 'bg-accent text-foreground'
				)}
				title={categoryLabel(e.action)}
			>
				<Icon class="size-[11px]" />
			</span>
		</span>
		<div class="flex min-w-0 flex-col gap-[3px]">
			<p class="flex flex-wrap items-center gap-x-1.5 gap-y-0.5">
				{@render sentence(e)}
				{#if e.sensitive}
					<span
						class="inline-flex h-5 items-center gap-1 rounded bg-amber-400/10 px-1.5 text-[11px] font-medium text-amber-400"
					>
						<LockIcon class="size-2.5" />Sensitive
					</span>
				{/if}
				{#if repeats.length}
					<button
						type="button"
						class="inline-flex h-5 items-center gap-1 rounded bg-accent px-1.5 text-[11px] font-medium text-muted-foreground hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:outline-none"
						aria-expanded={open}
						onclick={() => (open ? expanded.delete(e.id) : expanded.add(e.id))}
					>
						+{repeats.length} more since {clock.format(new Date(repeats.at(-1)!.created_at))}
						<ChevronDownIcon class={cn('size-[11px] transition-transform', open && 'rotate-180')} />
					</button>
				{/if}
			</p>
			<p class="flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground/80">
				<span class="shrink-0">{categoryLabel(e.action)}</span>
				{#each s.meta as m (m)}
					<span class="text-muted-foreground/40">·</span>
					<span class="truncate font-mono">{m}</span>
				{/each}
				{#if e.ip}
					<span class="text-muted-foreground/40 md:hidden">·</span>
					<span class="font-mono md:hidden">{e.ip}</span>
				{/if}
			</p>
		</div>
		<span class="hidden truncate font-mono text-xs text-muted-foreground/80 md:block" title={e.ip}
			>{e.ip || '–'}</span
		>
	</li>
	{#if open}
		{#each repeats as r (r.id)}
			{@render row(r, [], true)}
		{/each}
	{/if}
{/snippet}

<PageHeader title="Audit log" description="Who did what, and when. Newest first.">
	{#snippet aside()}
		{#if summary.data}
			<div class="flex flex-wrap items-center gap-x-5 gap-y-1 text-[13px] text-muted-foreground">
				<span class="flex items-center gap-2">
					<span class="size-2 rounded-full bg-muted-foreground/80"></span>{summary.data.events}
					{summary.data.events === 1 ? 'event' : 'events'} today
				</span>
				<button
					type="button"
					class="flex items-center gap-2 hover:text-foreground"
					onclick={() => (sensitiveOnly = true)}
					title="Show sensitive events only"
				>
					<span class="size-2 rounded-full bg-amber-400"></span>{summary.data.sensitive} sensitive
				</button>
			</div>
		{/if}
	{/snippet}
</PageHeader>

<div class="container-page flex flex-col gap-5 pt-8 pb-16">
	<div class="flex flex-col gap-3 md:flex-row md:items-center">
		<label class="relative min-w-0 flex-1">
			<span class="sr-only">Search the audit log</span>
			<SearchIcon
				class="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground/80"
			/>
			<input
				bind:this={searchInput}
				bind:value={search}
				type="search"
				placeholder="Search by person, project, target or IP…"
				class="h-10 w-full rounded-md border border-input bg-card pr-11 pl-[38px] text-sm outline-none placeholder:text-muted-foreground/80 focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/40"
			/>
			<kbd
				class="pointer-events-none absolute top-1/2 right-3 -translate-y-1/2 rounded bg-muted px-1.5 font-mono text-xs leading-4 text-muted-foreground"
				>/</kbd
			>
		</label>

		<div class="grid grid-cols-2 gap-3 md:flex">
			<DropdownMenu.Root>
				<DropdownMenu.Trigger class={trigger} aria-label="Activity type">
					<CategoryIcon
						class={cn(
							'size-[15px] shrink-0 text-muted-foreground',
							sensitiveOnly && 'text-amber-400'
						)}
					/>
					<span class="min-w-0 flex-1 truncate text-left"
						>{categoryName}{sensitiveOnly ? ' · sensitive' : ''}</span
					>
					<ChevronDownIcon class="size-3.5 shrink-0 text-muted-foreground/80" />
				</DropdownMenu.Trigger>
				<DropdownMenu.Content align="end" class="w-[260px] rounded-lg p-1.5">
					<DropdownMenu.Item class={menuItem} onSelect={() => (category = '')}>
						<LayersIcon />
						<span class="flex-1">All activity</span>
						{#if !category}<CheckIcon class="size-3.5!" />{/if}
					</DropdownMenu.Item>
					<DropdownMenu.Separator class="my-1" />
					{#each auditCategories as c (c.value)}
						{@const Icon = categoryIcons[c.value]}
						<DropdownMenu.Item class={menuItem} onSelect={() => (category = c.value)}>
							<Icon class="text-muted-foreground" />
							<span class="flex-1">{c.label}</span>
							{#if category === c.value}<CheckIcon class="size-3.5!" />{/if}
						</DropdownMenu.Item>
					{/each}
					<DropdownMenu.Separator class="my-1" />
					<DropdownMenuPrimitive.CheckboxItem
						bind:checked={sensitiveOnly}
						closeOnSelect={false}
						class="flex h-[38px] cursor-default items-center gap-2.5 rounded-md px-2.5 text-sm outline-none data-highlighted:bg-accent"
					>
						{#snippet children({ checked })}
							<LockIcon class="size-[15px] text-amber-400" />
							<span class="flex-1">Sensitive only</span>
							<span
								class={cn(
									'relative h-5 w-[34px] rounded-full border transition-colors',
									checked ? 'border-foreground bg-foreground' : 'border-input bg-accent'
								)}
								aria-hidden="true"
							>
								<span
									class={cn(
										'absolute top-0.5 size-3.5 rounded-full transition-all',
										checked ? 'left-4 bg-background' : 'left-0.5 bg-muted-foreground'
									)}
								></span>
							</span>
						{/snippet}
					</DropdownMenuPrimitive.CheckboxItem>
				</DropdownMenu.Content>
			</DropdownMenu.Root>

			<Select.Root type="single" bind:value={userFilter}>
				<Select.Trigger
					class={cn(trigger, 'h-10! justify-start [&>svg:last-child]:ml-auto')}
					aria-label="Person"
				>
					{#if actor}
						<UserAvatar user={actor} class="size-[18px] text-[8px]" />
						<span class="truncate">{actor.display_name || actor.username}</span>
					{:else}
						<span
							class="flex size-[18px] shrink-0 items-center justify-center rounded-full bg-accent"
						>
							<UsersIcon class="size-[11px] text-muted-foreground" />
						</span>
						<span>Everyone</span>
					{/if}
				</Select.Trigger>
				<Select.Content align="end">
					<Select.Item value="" label="Everyone">Everyone</Select.Item>
					{#each users.data ?? [] as u (u.id)}
						<Select.Item value={String(u.id)} label={u.display_name || u.username}>
							<UserAvatar user={u} class="size-[18px] text-[8px]" />
							{u.display_name || u.username}
						</Select.Item>
					{/each}
				</Select.Content>
			</Select.Root>
		</div>
	</div>

	{#if audit.isPending}
		<Skeleton class="h-96 w-full rounded-xl" />
	{:else if audit.isError}
		<Alert.Root variant="destructive">
			<CircleAlertIcon />
			<Alert.Title>{audit.error.message}</Alert.Title>
		</Alert.Root>
	{:else}
		<section class="overflow-hidden rounded-xl border bg-card" aria-label="Audit log">
			{#if entries.length === 0}
				<div class="flex flex-col items-center gap-3 px-6 py-14 text-center">
					<HistoryIcon class="size-5 text-muted-foreground" />
					<p class="text-sm text-muted-foreground">
						{filtered ? 'No activity matches these filters.' : 'Nothing has been recorded yet.'}
					</p>
					{#if filtered}
						<Button variant="outline" class="h-8 px-3 text-[13px]" onclick={clearFilters}>
							Clear filters
						</Button>
					{/if}
				</div>
			{:else}
				{#each days as day (day.key)}
					{@const title = dayTitle(day.key)}
					{@const count = dayCounts[day.key]}
					<div class="flex items-center gap-2 border-b bg-background px-4 py-2.5 text-xs md:px-5">
						{#if title}<span class="font-semibold">{title}</span>{/if}
						<span class={title ? 'text-muted-foreground/80' : 'font-semibold'}
							>{dayDate(day.date)}</span
						>
						{#if count !== undefined}
							<span class="ml-auto text-muted-foreground/80"
								>{count} {count === 1 ? 'event' : 'events'}</span
							>
						{/if}
					</div>
					<ul>
						{#each day.events as ev (ev.head.id)}
							{@render row(ev.head, ev.repeats)}
						{/each}
					</ul>
				{/each}
				<div
					class="-mt-px flex items-center justify-between gap-3 border-t bg-background px-4 py-3 md:px-5"
				>
					<p class="text-[13px] text-muted-foreground/80">
						Showing {entries.length} of {total}
						{total === 1 ? 'event' : 'events'}
					</p>
					{#if audit.hasNextPage}
						<Button
							variant="outline"
							class="h-8 px-3 text-[13px]"
							onclick={() => audit.fetchNextPage()}
							disabled={audit.isFetchingNextPage}
						>
							{#if audit.isFetchingNextPage}<LoaderIcon class="animate-spin" />{:else}<ArrowDownIcon
								/>{/if}
							Load older
						</Button>
					{/if}
				</div>
			{/if}
		</section>
	{/if}
</div>

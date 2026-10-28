<script lang="ts">
	import { createQuery } from '@tanstack/svelte-query';
	import { resolve } from '$app/paths';
	import { api } from '#lib/api/client.js';
	import { roleKeys, rolesApi } from '#lib/api/users.js';
	import type { Action, AuthProviders, RoleSummary } from '#lib/api/types.gen.js';
	import PageHeader from '#lib/components/page-header.svelte';
	import { roleInfo } from '#lib/components/role-picker.svelte';
	import UserAvatar from '#lib/components/user-avatar.svelte';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import * as Alert from '#lib/components/ui/alert/index.js';
	import { cn } from '#lib/utils.js';
	import ArrowRightIcon from '@lucide/svelte/icons/arrow-right';
	import CheckIcon from '@lucide/svelte/icons/check';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import InfoIcon from '@lucide/svelte/icons/info';
	import LockIcon from '@lucide/svelte/icons/lock';
	import MinusIcon from '@lucide/svelte/icons/minus';

	const roles = createQuery(() => ({ queryKey: roleKeys.all, queryFn: rolesApi.list }));
	const providers = createQuery(() => ({
		queryKey: ['auth', 'providers'],
		queryFn: () => api.get<AuthProviders>('/auth/providers'),
		staleTime: Infinity
	}));

	const signInMethods = $derived.by(() => {
		const p = providers.data;
		const names = [p?.local && 'Password', p?.oidc.enabled && p.oidc.name].filter(Boolean);
		return names.length ? names.join(' and ') : 'Password and SSO';
	});

	// Each row needs every listed action; the checks come from the server's RBAC table.
	type Row = { title: string; description: string; actions: Action[] };
	const groups = $derived<{ label: string; rows: Row[] }[]>([
		{
			label: 'Projects & deployments',
			rows: [
				{
					title: 'View projects & deployments',
					description: 'Project list, history and status',
					actions: ['project.view']
				},
				{
					title: 'Read build logs',
					description: 'Live and past deployment output',
					actions: ['project.view']
				},
				{
					title: 'Deploy, cancel & redeploy',
					description: 'Start or stop builds by hand',
					actions: ['project.deploy']
				}
			]
		},
		{
			label: 'Configuration',
			rows: [
				{
					title: 'Create & delete projects',
					description: 'Connect new repositories',
					actions: ['project.create', 'project.edit']
				},
				{
					title: 'Edit triggers & deploy scripts',
					description: 'Change what runs, and when',
					actions: ['project.edit']
				},
				{
					title: 'Manage secrets & environment',
					description: 'Read and write env variables',
					actions: ['project.secrets']
				}
			]
		},
		{
			label: 'Administration',
			rows: [
				{
					title: 'Manage users & reset passwords',
					description: 'Everything on the Users page',
					actions: ['user.manage']
				},
				{
					title: 'Configure sign-in methods',
					description: signInMethods,
					actions: ['user.manage']
				},
				{ title: 'View audit log', description: 'Who did what, and when', actions: ['audit.view'] }
			]
		}
	]);

	const allows = (r: RoleSummary, row: Row) => row.actions.every((a) => r.permissions.includes(a));

	const cols =
		'grid grid-cols-[minmax(0,1fr)_repeat(3,3.5rem)] sm:grid-cols-[minmax(0,1fr)_repeat(3,6rem)] md:grid-cols-[minmax(0,2fr)_repeat(3,minmax(0,1fr))]';
	const roleCell = 'flex items-center justify-center border-l';
</script>

<PageHeader
	title="Roles"
	description="Built-in roles that decide what each user can do, across every project."
>
	{#snippet aside()}
		<span
			class="inline-flex h-6 items-center gap-[5px] rounded-[5px] border border-input bg-muted px-[7px] text-xs text-muted-foreground"
		>
			<LockIcon class="size-3" />
			Built-in · read-only
		</span>
	{/snippet}
</PageHeader>

<div class="container-page flex flex-col gap-4 pt-8 pb-16">
	{#if roles.isPending}
		<Skeleton class="h-[56rem] w-full rounded-xl" />
	{:else if roles.isError}
		<Alert.Root variant="destructive">
			<CircleAlertIcon />
			<Alert.Title>{roles.error.message}</Alert.Title>
		</Alert.Root>
	{:else}
		<section class="overflow-hidden rounded-xl border bg-card" aria-labelledby="compare-roles">
			<div class={cn(cols, 'border-b')}>
				<div class="flex flex-col justify-end gap-1.5 p-4 md:p-6">
					<h2 id="compare-roles" class="text-base font-semibold">Compare roles</h2>
					<p class="hidden text-[13px] text-muted-foreground/80 sm:block">
						Assign a role to each person on the
						<a
							href={resolve('/(app)/users')}
							class="underline-offset-2 hover:text-foreground hover:underline">Users</a
						> page.
					</p>
				</div>
				{#each roles.data as r (r.role)}
					{@const info = roleInfo(r.role)}
					<div
						class="flex min-w-0 flex-col items-center gap-2 border-l px-1 py-4 md:items-stretch md:gap-3.5 md:p-6"
					>
						<span
							class="flex size-8 shrink-0 items-center justify-center rounded-lg border border-input bg-muted"
						>
							<info.icon class="size-[15px]" />
						</span>
						<div class="flex min-w-0 flex-col gap-1">
							<h3 class="text-xs font-semibold md:text-base">{info.label}</h3>
							<p class="hidden text-[13px] text-muted-foreground md:block">{info.description}</p>
						</div>
						<div
							class="hidden items-center gap-2 md:flex"
							title={r.members
								.map((m) => `${m.display_name || m.username}${m.disabled ? ' (disabled)' : ''}`)
								.join(', ')}
						>
							{#if r.members.length}
								<span class="flex shrink-0 -space-x-1.5">
									{#each r.members.slice(0, 4) as m (m.id)}
										<UserAvatar
											user={m}
											class={cn(
												'size-[22px] text-[9px] ring-2 ring-card',
												m.disabled && 'opacity-50'
											)}
										/>
									{/each}
								</span>
							{/if}
							<span class="truncate text-xs text-muted-foreground/80">
								{#if r.members.length === 0}
									No members
								{:else}
									{r.members.length}
									{r.members.length === 1 ? 'member' : 'members'}
								{/if}
							</span>
						</div>
					</div>
				{/each}
			</div>

			{#each groups as g (g.label)}
				<div class={cn(cols, 'border-b bg-background')}>
					<span class="px-4 py-2.5 text-xs font-medium text-muted-foreground/80 md:px-6"
						>{g.label}</span
					>
					{#each roles.data as r (r.role)}<span class="border-l"></span>{/each}
				</div>
				{#each g.rows as row (row.title)}
					<div class={cn(cols, 'border-b')}>
						<div class="flex min-w-0 flex-col gap-0.5 px-4 py-[13px] md:px-6">
							<span class="text-sm font-medium">{row.title}</span>
							<span class="text-xs text-muted-foreground/80">{row.description}</span>
						</div>
						{#each roles.data as r (r.role)}
							{@const label = roleInfo(r.role).label}
							<span class={roleCell}>
								{#if allows(r, row)}
									<CheckIcon class="size-4 text-emerald-400" aria-label={`${label}: allowed`} />
								{:else}
									<MinusIcon
										class="size-4 text-muted-foreground/30"
										aria-label={`${label}: not allowed`}
									/>
								{/if}
							</span>
						{/each}
					</div>
				{/each}
			{/each}

			<div
				class="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 bg-background px-4 py-3.5 md:px-6"
			>
				<p class="flex items-start gap-2.5 text-[13px] text-muted-foreground">
					<InfoIcon class="mt-0.5 size-[15px] shrink-0 text-muted-foreground/80" />
					At least one active Admin is always required. The last Admin can't be disabled or demoted.
				</p>
				<a
					href={resolve('/(app)/users')}
					class="flex items-center gap-1 text-[13px] text-muted-foreground hover:text-foreground"
				>
					Manage users
					<ArrowRightIcon class="size-3.5" />
				</a>
			</div>
		</section>
	{/if}
</div>

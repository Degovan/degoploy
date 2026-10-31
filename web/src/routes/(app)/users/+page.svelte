<script lang="ts">
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { toast } from 'svelte-sonner';
	import { api, errorMessage } from '#lib/api/client.js';
	import { emailApi, emailKeys } from '#lib/api/email.js';
	import { inviteKeys, invitesApi, userKeys, usersApi } from '#lib/api/users.js';
	import type {
		AuthProviders,
		Invite,
		InviteLink,
		Me,
		Role,
		UpdateUserRequest,
		User
	} from '#lib/api/types.gen.js';
	import InviteDialog from '#lib/components/invite-dialog.svelte';
	import PageHeader from '#lib/components/page-header.svelte';
	import { roleInfo, roles } from '#lib/components/role-picker.svelte';
	import ResetPasswordDialog from '#lib/components/reset-password-dialog.svelte';
	import UserAvatar from '#lib/components/user-avatar.svelte';
	import UserDialog, { type UserFormValues } from '#lib/components/user-dialog.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import * as Alert from '#lib/components/ui/alert/index.js';
	import * as AlertDialog from '#lib/components/ui/alert-dialog/index.js';
	import * as Tooltip from '#lib/components/ui/tooltip/index.js';
	import { session } from '#lib/stores/session.svelte.js';
	import { formatDateTime, timeAgo } from '#lib/time.js';
	import { cn } from '#lib/utils.js';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import FingerprintIcon from '@lucide/svelte/icons/fingerprint-pattern';
	import KeyRoundIcon from '@lucide/svelte/icons/key-round';
	import LoaderIcon from '@lucide/svelte/icons/loader-circle';
	import MailIcon from '@lucide/svelte/icons/mail';
	import MailPlusIcon from '@lucide/svelte/icons/mail-plus';
	import PencilIcon from '@lucide/svelte/icons/pencil';
	import SearchIcon from '@lucide/svelte/icons/search';
	import SendIcon from '@lucide/svelte/icons/send';
	import XIcon from '@lucide/svelte/icons/x';

	const queryClient = useQueryClient();
	const users = createQuery(() => ({ queryKey: userKeys.all, queryFn: usersApi.list }));
	const invites = createQuery(() => ({ queryKey: inviteKeys.all, queryFn: invitesApi.list }));
	const providers = createQuery(() => ({
		queryKey: ['auth', 'providers'],
		queryFn: () => api.get<AuthProviders>('/auth/providers'),
		staleTime: Infinity
	}));
	const refresh = () => queryClient.invalidateQueries({ queryKey: userKeys.all });
	const refreshInvites = () => queryClient.invalidateQueries({ queryKey: inviteKeys.all });

	const ssoName = $derived(providers.data?.oidc.name || 'SSO');
	// Invites create password accounts, so they need local login.
	const canInvite = $derived(providers.data?.local ?? true);
	// Resending emails a new link, so it needs working email.
	const email = createQuery(() => ({ queryKey: emailKeys.status, queryFn: emailApi.status }));
	const canResend = $derived(canInvite && !!email.data?.ready);

	// One table: active users, then pending invites, then disabled users; you come first.
	type Row =
		{ kind: 'user'; id: string; user: User } | { kind: 'invite'; id: string; invite: Invite };
	const rows = $derived.by((): Row[] => {
		const list = users.data ?? [];
		const self = list.filter((u) => u.id === session.user?.id);
		const others = list.filter((u) => u.id !== session.user?.id);
		const asRow = (u: User): Row => ({ kind: 'user', id: `u${u.id}`, user: u });
		return [
			...[...self, ...others.filter((u) => !u.disabled)].map(asRow),
			...(invites.data ?? []).map((i): Row => ({ kind: 'invite', id: `i${i.id}`, invite: i })),
			...others.filter((u) => u.disabled).map(asRow)
		];
	});
	const roleOf = (r: Row) => (r.kind === 'user' ? r.user.role : r.invite.role);

	const summary = $derived({
		active: (users.data ?? []).filter((u) => !u.disabled).length,
		invited: (invites.data ?? []).length,
		disabled: (users.data ?? []).filter((u) => u.disabled).length
	});

	let query = $state('');
	let roleFilter = $state<Role | 'all'>('all');
	let searchInput = $state<HTMLInputElement | null>(null);

	const counts = $derived(
		Object.fromEntries([
			['all', rows.length],
			...roles.map((r) => [r.value, rows.filter((row) => roleOf(row) === r.value).length])
		]) as Record<Role | 'all', number>
	);
	const filtered = $derived.by(() => {
		const q = query.trim().toLowerCase();
		return rows.filter((r) => {
			if (roleFilter !== 'all' && roleOf(r) !== roleFilter) return false;
			if (!q) return true;
			const hay =
				r.kind === 'user'
					? [r.user.username, r.user.display_name, r.user.email]
					: [r.invite.email, r.invite.invited_by];
			return hay.some((s) => s.toLowerCase().includes(q));
		});
	});

	function onKeydown(e: KeyboardEvent) {
		if (e.key !== '/' || e.metaKey || e.ctrlKey || e.altKey) return;
		const el = e.target as HTMLElement;
		if (el.closest('input, textarea, select, [contenteditable="true"], [role="dialog"]')) return;
		e.preventDefault();
		searchInput?.focus();
	}

	// Edit
	let dialogOpen = $state(false);
	let editing = $state<User | null>(null);

	async function save(v: UserFormValues) {
		const u = editing!;
		const body: UpdateUserRequest = { display_name: v.display_name, email: v.email };
		if (u.id !== session.user?.id) {
			body.role = v.role;
			body.disabled = v.disabled;
		}
		if (v.role !== 'admin') {
			body.selected_projects = v.selected_projects;
			if (v.selected_projects) body.projects = v.projects;
		}
		await usersApi.update(u.id, body);
		toast.success(`Saved ${u.username}`);
		// Keep the header's name/email in sync when editing yourself.
		if (u.id === session.user?.id) session.set(await api.get<Me>('/me'));
		await refresh();
	}

	// Reset password
	let pwOpen = $state(false);
	let pwUser = $state<User | null>(null);

	// Invites
	let inviteOpen = $state(false);
	let inviteLinks = $state<InviteLink[] | null>(null);
	let renewing = $state<number | null>(null);

	async function resend(inv: Invite) {
		renewing = inv.id;
		try {
			inviteLinks = [await invitesApi.renew(inv.id)];
			inviteOpen = true;
			await refreshInvites();
		} catch (err) {
			toast.error(errorMessage(err));
		} finally {
			renewing = null;
		}
	}

	// Confirmation for deleting a user or revoking an invite. `confirm` outlives
	// the dialog's close animation.
	type Confirm = { title: string; description: string; action: string; run: () => Promise<void> };
	let confirm = $state<Confirm | null>(null);
	let confirmOpen = $state(false);
	let confirmBusy = $state(false);

	function ask(c: Confirm) {
		confirm = c;
		confirmOpen = true;
	}
	async function runConfirm() {
		if (!confirm) return;
		confirmBusy = true;
		try {
			await confirm.run();
			confirmOpen = false;
		} catch (err) {
			toast.error(errorMessage(err));
		} finally {
			confirmBusy = false;
		}
	}

	const askDelete = (u: User) =>
		ask({
			title: `Delete ${u.display_name || u.username}?`,
			description:
				'Their sessions end immediately and they can no longer sign in. Audit entries and deployments they triggered are kept without the user link.',
			action: 'Delete user',
			run: async () => {
				await usersApi.remove(u.id);
				toast.success(`Deleted ${u.username}`);
				await refresh();
			}
		});
	const askRevoke = (inv: Invite) =>
		ask({
			title: `Revoke the invite for ${inv.email}?`,
			description: 'The link stops working at once. You can invite them again later.',
			action: 'Revoke invite',
			run: async () => {
				await invitesApi.revoke(inv.id);
				toast.success(`Revoked the invite for ${inv.email}`);
				await refreshInvites();
			}
		});

	const capitalize = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);
	function expiresIn(iso: string) {
		const ms = new Date(iso).getTime() - Date.now();
		const h = Math.max(1, Math.round(ms / 3_600_000));
		return h >= 24 ? `${Math.round(h / 24)}d` : `${h}h`;
	}

	const cols =
		'grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-4 gap-y-2.5 md:grid-cols-[minmax(0,1fr)_130px_140px_110px_120px_64px]';
	const iconBtn =
		'flex size-[30px] items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none aria-disabled:cursor-not-allowed aria-disabled:opacity-30 aria-disabled:hover:bg-transparent [&_svg]:size-[15px]';
</script>

<svelte:window onkeydown={onKeydown} />

{#snippet roleTag(role: Role)}
	{@const info = roleInfo(role)}
	<span
		class="inline-flex h-[22px] items-center gap-[5px] rounded-[5px] border border-input bg-muted px-[7px] text-xs font-medium"
	>
		<info.icon
			class={cn('size-3', role === 'admin' ? 'text-foreground' : 'text-muted-foreground')}
		/>
		{info.label}
	</span>
{/snippet}

{#snippet status(dot: string, label: string, muted = false)}
	<span class={cn('inline-flex items-center gap-2 text-[13px]', muted && 'text-muted-foreground')}>
		<span class={cn('size-2 shrink-0 rounded-full', dot)}></span>{label}
	</span>
{/snippet}

{#snippet action(
	label: string,
	onclick: () => void,
	Icon: typeof PencilIcon,
	disabled = false,
	hint = ''
)}
	<Tooltip.Root>
		<Tooltip.Trigger>
			{#snippet child({ props })}
				<!-- aria-disabled rather than disabled, so the tooltip still explains why. -->
				<button
					{...props}
					type="button"
					class={iconBtn}
					aria-label={label}
					aria-disabled={disabled}
					onclick={() => !disabled && onclick()}
				>
					<Icon />
				</button>
			{/snippet}
		</Tooltip.Trigger>
		<Tooltip.Content>{hint || label}</Tooltip.Content>
	</Tooltip.Root>
{/snippet}

<PageHeader
	title="Users"
	description="Who can sign in to degoploy, and what they're allowed to do."
>
	{#snippet aside()}
		{#if users.data}
			<div class="flex flex-wrap items-center gap-x-5 gap-y-1 text-[13px] text-muted-foreground">
				<span class="flex items-center gap-2">
					<span class="size-2 rounded-full bg-emerald-400"></span>{summary.active} active
				</span>
				{#if summary.invited}
					<span class="flex items-center gap-2">
						<span class="size-2 rounded-full bg-amber-400"></span>{summary.invited} invited
					</span>
				{/if}
				{#if summary.disabled}
					<span class="flex items-center gap-2">
						<span class="size-2 rounded-full bg-zinc-500"></span>{summary.disabled} disabled
					</span>
				{/if}
			</div>
		{/if}
	{/snippet}
</PageHeader>

<div class="container-page flex flex-col gap-6 pt-8 pb-16">
	<div class="flex flex-wrap items-center gap-3">
		<label class="relative min-w-60 flex-1">
			<span class="sr-only">Search users</span>
			<SearchIcon
				class="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground/80"
			/>
			<input
				bind:this={searchInput}
				bind:value={query}
				type="search"
				placeholder="Search by name, username or email…"
				class="h-10 w-full rounded-md border border-input bg-card pr-11 pl-[38px] text-sm outline-none placeholder:text-muted-foreground/80 focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/40"
			/>
			<kbd
				class="pointer-events-none absolute top-1/2 right-3 -translate-y-1/2 rounded bg-muted px-1.5 font-mono text-xs leading-4 text-muted-foreground"
				>/</kbd
			>
		</label>
		<div
			class="flex h-10 items-center gap-0.5 overflow-x-auto rounded-md border border-input p-[3px]"
			role="group"
			aria-label="Filter by role"
		>
			{#each [{ value: 'all', label: 'All' } as const, ...roles] as r (r.value)}
				{@const active = roleFilter === r.value}
				<button
					type="button"
					class={cn(
						'flex h-full items-center gap-1.5 rounded-sm px-3 text-[13px] font-medium whitespace-nowrap transition-colors outline-none focus-visible:ring-3 focus-visible:ring-ring/50',
						active ? 'bg-accent text-foreground' : 'text-muted-foreground hover:text-foreground'
					)}
					aria-pressed={active}
					onclick={() => (roleFilter = r.value)}
				>
					{r.label}
					<span
						class={cn(
							'font-mono text-xs font-normal',
							active ? 'text-muted-foreground' : 'text-muted-foreground/80'
						)}>{counts[r.value]}</span
					>
				</button>
			{/each}
		</div>
		{#if canInvite}
			<Button
				class="h-10 px-3.5 text-[13px]"
				onclick={() => {
					inviteLinks = null;
					inviteOpen = true;
				}}
			>
				<MailPlusIcon />
				Invite user
			</Button>
		{/if}
	</div>

	{#if users.isPending}
		<Skeleton class="h-80 w-full rounded-xl" />
	{:else if users.isError}
		<Alert.Root variant="destructive">
			<CircleAlertIcon />
			<Alert.Title>{users.error.message}</Alert.Title>
		</Alert.Root>
	{:else}
		<Tooltip.Provider delayDuration={300}>
			<section class="overflow-hidden rounded-xl border bg-card" aria-label="Users">
				<div
					class={cn(
						cols,
						'hidden border-b bg-background px-5 py-[11px] text-xs font-medium text-muted-foreground/80 md:grid'
					)}
					aria-hidden="true"
				>
					<span>User</span>
					<span>Role</span>
					<span>Sign-in</span>
					<span>Status</span>
					<span>Last login</span>
					<span></span>
				</div>

				{#if invites.isError}
					<p
						class="flex items-center gap-2 border-b px-5 py-3 text-[13px] text-destructive"
						role="alert"
					>
						<CircleAlertIcon class="size-3.5" />
						Could not load invites: {invites.error.message}
					</p>
				{/if}

				{#if filtered.length === 0}
					<p class="px-5 py-12 text-center text-sm text-muted-foreground">
						{query.trim() ? `No users match “${query.trim()}”.` : 'No users with this role.'}
					</p>
				{:else}
					<ul>
						{#each filtered as row (row.id)}
							<li class={cn(cols, 'border-b px-5 py-3.5 last:border-b-0')}>
								{#if row.kind === 'user'}
									{@const u = row.user}
									{@const self = u.id === session.user?.id}
									{@const dim = u.disabled && 'opacity-50'}
									<div
										class={cn('flex min-w-0 items-center gap-3', dim)}
										data-testid={`user-${u.username}`}
									>
										<UserAvatar user={u} />
										<div class="flex min-w-0 flex-col gap-[3px]">
											<span class="flex min-w-0 items-center gap-2">
												<span class="truncate text-sm font-medium"
													>{u.display_name || u.username}</span
												>
												{#if self}
													<span
														class="rounded bg-accent px-1.5 text-[11px] leading-[18px] font-medium text-muted-foreground"
														>You</span
													>
												{/if}
											</span>
											<span class="flex min-w-0 items-center gap-1.5 text-muted-foreground/80">
												<span class="shrink-0 font-mono text-xs">@{u.username}</span>
												{#if u.email}
													<span class="text-xs text-muted-foreground/40">·</span>
													<span class="truncate text-[13px]">{u.email}</span>
												{/if}
											</span>
										</div>
									</div>

									<div
										class="col-span-full flex flex-wrap items-center gap-x-4 gap-y-2 pl-11 md:contents"
									>
										<div class={cn('flex flex-col items-start gap-[3px]', dim)}>
											{@render roleTag(u.role)}
											{#if u.selected_projects}
												<span class="text-xs text-muted-foreground/80">
													{u.projects.length}
													{u.projects.length === 1 ? 'project' : 'projects'}
												</span>
											{/if}
										</div>
										<span
											class={cn(
												'flex min-w-0 items-center gap-2 text-[13px] text-muted-foreground',
												dim
											)}
										>
											{#if u.auth_provider === 'oidc'}
												<FingerprintIcon class="size-3.5 shrink-0 text-muted-foreground/80" />
												<span class="truncate">{ssoName}</span>
											{:else}
												<KeyRoundIcon class="size-3.5 shrink-0 text-muted-foreground/80" />
												Password
											{/if}
										</span>
										<div>
											{#if u.disabled}
												{@render status('bg-zinc-500', 'Disabled', true)}
											{:else}
												{@render status('bg-emerald-400', 'Active')}
											{/if}
										</div>
										<span class="flex flex-col gap-[3px] text-[13px] text-muted-foreground">
											{#if u.last_login_at}
												<time datetime={u.last_login_at} title={formatDateTime(u.last_login_at)}
													>{capitalize(timeAgo(u.last_login_at))}</time
												>
											{:else}
												<span class="text-muted-foreground/80">Never</span>
											{/if}
											{#if u.must_change_password}
												<span class="text-xs text-amber-400">Must set a new password</span>
											{/if}
										</span>
									</div>

									<div
										class="col-start-2 row-start-1 flex items-center justify-end gap-1 md:col-start-auto md:row-start-auto"
									>
										{@render action(
											`Edit ${u.username}`,
											() => {
												editing = u;
												dialogOpen = true;
											},
											PencilIcon
										)}
										{@render action(
											`Reset password for ${u.username}`,
											() => {
												pwUser = u;
												pwOpen = true;
											},
											KeyRoundIcon,
											u.auth_provider !== 'local',
											u.auth_provider === 'local'
												? 'Reset password'
												: `Signs in with ${ssoName}; no password to reset`
										)}
									</div>
								{:else}
									{@const inv = row.invite}
									<div class="flex min-w-0 items-center gap-3">
										<span
											class="flex size-8 shrink-0 items-center justify-center rounded-full border border-input bg-muted text-muted-foreground/80"
											aria-hidden="true"
										>
											<MailIcon class="size-[15px]" />
										</span>
										<div class="flex min-w-0 flex-col gap-[3px]">
											<span class="truncate text-sm font-medium">{inv.email}</span>
											<span class="flex min-w-0 items-center gap-1.5 text-muted-foreground/80">
												<span class="shrink-0 font-mono text-xs"
													>{inv.invited_by ? `Invited by @${inv.invited_by}` : 'Invited'}</span
												>
												<span class="text-xs text-muted-foreground/40">·</span>
												<time
													class="truncate text-[13px]"
													datetime={inv.created_at}
													title={formatDateTime(inv.created_at)}>{timeAgo(inv.created_at)}</time
												>
											</span>
										</div>
									</div>

									<div
										class="col-span-full flex flex-wrap items-center gap-x-4 gap-y-2 pl-11 md:contents"
									>
										<div>{@render roleTag(inv.role)}</div>
										<span class="flex items-center gap-2 text-[13px] text-muted-foreground">
											<KeyRoundIcon class="size-3.5 shrink-0 text-muted-foreground/80" />
											Password
										</span>
										<div>
											{#if inv.expired}
												{@render status('bg-red-500', 'Expired')}
											{:else}
												{@render status('bg-amber-400', 'Invited')}
											{/if}
										</div>
										<span class="flex flex-col gap-[3px] text-muted-foreground/80">
											<span class="text-[13px]">Never</span>
											<span class="text-xs" title={formatDateTime(inv.expires_at)}>
												{inv.expired
													? 'Invite expired'
													: `Invite expires in ${expiresIn(inv.expires_at)}`}
											</span>
										</span>
									</div>

									<div
										class="col-start-2 row-start-1 flex items-center justify-end gap-1 md:col-start-auto md:row-start-auto"
									>
										{#if renewing === inv.id}
											<span class={iconBtn}><LoaderIcon class="animate-spin" /></span>
										{:else}
											{@render action(
												`Resend invite to ${inv.email}`,
												() => resend(inv),
												SendIcon,
												!canResend,
												canResend
													? 'Email a new invite link'
													: 'Set up email on the Email page to resend invites'
											)}
										{/if}
										{@render action(`Revoke invite for ${inv.email}`, () => askRevoke(inv), XIcon)}
									</div>
								{/if}
							</li>
						{/each}
					</ul>
				{/if}
			</section>
		</Tooltip.Provider>
	{/if}
</div>

<UserDialog
	bind:open={dialogOpen}
	user={editing}
	isSelf={editing?.id === session.user?.id}
	{ssoName}
	onsubmit={save}
	ondelete={askDelete}
/>

<InviteDialog bind:open={inviteOpen} bind:links={inviteLinks} oncreated={refreshInvites} />

<ResetPasswordDialog bind:open={pwOpen} user={pwUser} isSelf={pwUser?.id === session.user?.id} />

<AlertDialog.Root open={confirmOpen} onOpenChange={(o) => (confirmOpen = o || confirmBusy)}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>{confirm?.title}</AlertDialog.Title>
			<AlertDialog.Description>{confirm?.description}</AlertDialog.Description>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel disabled={confirmBusy}>Cancel</AlertDialog.Cancel>
			<Button variant="destructive" onclick={runConfirm} disabled={confirmBusy}>
				{#if confirmBusy}<LoaderIcon class="animate-spin" />{/if}
				{confirm?.action}
			</Button>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>

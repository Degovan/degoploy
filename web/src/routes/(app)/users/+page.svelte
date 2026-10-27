<script lang="ts">
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { toast } from 'svelte-sonner';
	import { api, errorMessage } from '#lib/api/client.js';
	import { userKeys, usersApi } from '#lib/api/users.js';
	import type { Me, UpdateUserRequest, User } from '#lib/api/types.gen.js';
	import PageHeader from '#lib/components/page-header.svelte';
	import ConfirmButton from '#lib/components/confirm-button.svelte';
	import UserDialog, { roles, type UserFormValues } from '#lib/components/user-dialog.svelte';
	import SetPasswordDialog from '#lib/components/set-password-dialog.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Badge } from '#lib/components/ui/badge/index.js';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import * as Avatar from '#lib/components/ui/avatar/index.js';
	import * as Card from '#lib/components/ui/card/index.js';
	import * as Table from '#lib/components/ui/table/index.js';
	import * as Alert from '#lib/components/ui/alert/index.js';
	import { session } from '#lib/stores/session.svelte.js';
	import { formatDateTime, timeAgo } from '#lib/time.js';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import KeyRoundIcon from '@lucide/svelte/icons/key-round';
	import PencilIcon from '@lucide/svelte/icons/pencil';
	import Trash2Icon from '@lucide/svelte/icons/trash-2';
	import UserPlusIcon from '@lucide/svelte/icons/user-plus';

	const queryClient = useQueryClient();
	const users = createQuery(() => ({ queryKey: userKeys.all, queryFn: usersApi.list }));
	const refresh = () => queryClient.invalidateQueries({ queryKey: userKeys.all });

	let dialogOpen = $state(false);
	let editing = $state<User | null>(null);
	let pwOpen = $state(false);
	let pwUser = $state<User | null>(null);

	const roleLabel = (r: string) => roles.find((x) => x.value === r)?.label ?? r;
	const initials = (u: User) =>
		(u.display_name || u.username)
			.split(/[\s._-]+/)
			.filter(Boolean)
			.slice(0, 2)
			.map((p) => p[0]!.toUpperCase())
			.join('');

	async function save(v: UserFormValues) {
		if (editing) {
			const body: UpdateUserRequest = { display_name: v.display_name, email: v.email };
			if (editing.id !== session.user?.id) {
				body.role = v.role;
				body.disabled = v.disabled;
			}
			await usersApi.update(editing.id, body);
			toast.success(`Saved ${editing.username}`);
			// Keep the sidebar's name/email in sync when editing yourself.
			if (editing.id === session.user?.id) session.set(await api.get<Me>('/me'));
		} else {
			await usersApi.create({
				username: v.username,
				display_name: v.display_name,
				email: v.email,
				role: v.role,
				password: v.password
			});
			toast.success(`Added ${v.username}`);
		}
		await refresh();
	}

	async function remove(u: User) {
		try {
			await usersApi.remove(u.id);
			toast.success(`Deleted ${u.username}`);
			await refresh();
		} catch (err) {
			toast.error(errorMessage(err));
		}
	}
</script>

<PageHeader title="Users" description="Who can sign in and what they may do.">
	{#snippet actions()}
		<Button
			onclick={() => {
				editing = null;
				dialogOpen = true;
			}}
		>
			<UserPlusIcon />
			Add user
		</Button>
	{/snippet}
</PageHeader>

<div class="container-page flex flex-col gap-6 py-8">
	{#if users.isPending}
		<Skeleton class="h-64 w-full rounded-xl" />
	{:else if users.isError}
		<Alert.Root variant="destructive">
			<CircleAlertIcon />
			<Alert.Title>{users.error.message}</Alert.Title>
		</Alert.Root>
	{:else}
		<Card.Root class="py-0">
			<Table.Root>
				<Table.Header>
					<Table.Row>
						<Table.Head class="pl-6">User</Table.Head>
						<Table.Head>Role</Table.Head>
						<Table.Head>Sign-in</Table.Head>
						<Table.Head>Status</Table.Head>
						<Table.Head>Last login</Table.Head>
						<Table.Head class="pr-6 text-right"><span class="sr-only">Actions</span></Table.Head>
					</Table.Row>
				</Table.Header>
				<Table.Body>
					{#each users.data as u (u.id)}
						{@const self = u.id === session.user?.id}
						<Table.Row data-testid={`user-${u.username}`}>
							<Table.Cell class="pl-6">
								<div class="flex items-center gap-3">
									<Avatar.Root class="size-8">
										<Avatar.Fallback>{initials(u)}</Avatar.Fallback>
									</Avatar.Root>
									<div class="flex flex-col">
										<span class="flex items-center gap-2 font-medium">
											{u.display_name || u.username}
											{#if self}<Badge variant="secondary">You</Badge>{/if}
										</span>
										<span class="text-xs text-muted-foreground">
											@{u.username}{u.email ? ` · ${u.email}` : ''}
										</span>
									</div>
								</div>
							</Table.Cell>
							<Table.Cell><Badge variant="outline">{roleLabel(u.role)}</Badge></Table.Cell>
							<Table.Cell class="text-muted-foreground">
								{u.auth_provider === 'oidc' ? 'SSO' : 'Password'}
							</Table.Cell>
							<Table.Cell>
								{#if u.disabled}
									<Badge variant="secondary">Disabled</Badge>
								{:else}
									<span class="text-sm text-emerald-700 dark:text-emerald-400">Active</span>
								{/if}
							</Table.Cell>
							<Table.Cell class="text-muted-foreground">
								{#if u.last_login_at}
									<time datetime={u.last_login_at} title={formatDateTime(u.last_login_at)}
										>{timeAgo(u.last_login_at)}</time
									>
								{:else}
									Never
								{/if}
							</Table.Cell>
							<Table.Cell class="pr-6">
								<div class="flex justify-end gap-1">
									<Button
										variant="ghost"
										size="icon-sm"
										aria-label={`Edit ${u.username}`}
										onclick={() => {
											editing = u;
											dialogOpen = true;
										}}><PencilIcon /></Button
									>
									{#if u.auth_provider === 'local'}
										<Button
											variant="ghost"
											size="icon-sm"
											aria-label={`Reset password for ${u.username}`}
											onclick={() => {
												pwUser = u;
												pwOpen = true;
											}}><KeyRoundIcon /></Button
										>
									{/if}
									{#if !self}
										<ConfirmButton
											variant="ghost"
											size="icon-sm"
											label={`Delete ${u.username}`}
											title={`Delete ${u.username}?`}
											description="Their sessions end immediately. Audit entries and deployments they triggered are kept without the user link."
											confirmLabel="Delete user"
											destructive
											onconfirm={() => remove(u)}
										>
											<Trash2Icon />
										</ConfirmButton>
									{/if}
								</div>
							</Table.Cell>
						</Table.Row>
					{/each}
				</Table.Body>
			</Table.Root>
		</Card.Root>
	{/if}

	<UserDialog
		bind:open={dialogOpen}
		user={editing}
		isSelf={editing?.id === session.user?.id}
		onsubmit={save}
	/>

	<SetPasswordDialog
		bind:open={pwOpen}
		username={pwUser?.username}
		onsubmit={async (_current, next) => {
			await usersApi.setPassword(pwUser!.id, next);
			toast.success(`Password set for ${pwUser!.username}`);
		}}
	/>
</div>

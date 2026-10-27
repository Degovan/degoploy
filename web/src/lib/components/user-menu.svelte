<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { useQueryClient } from '@tanstack/svelte-query';
	import { setMode, userPrefersMode } from 'mode-watcher';
	import { toast } from 'svelte-sonner';
	import * as DropdownMenu from '#lib/components/ui/dropdown-menu/index.js';
	import ProjectAvatar from '#lib/components/project-avatar.svelte';
	import SetPasswordDialog from '#lib/components/set-password-dialog.svelte';
	import { session } from '#lib/stores/session.svelte.js';
	import { realtime } from '#lib/realtime.svelte.js';
	import { usersApi } from '#lib/api/users.js';
	import LogOutIcon from '@lucide/svelte/icons/log-out';
	import SunMoonIcon from '@lucide/svelte/icons/sun-moon';
	import KeyRoundIcon from '@lucide/svelte/icons/key-round';

	const queryClient = useQueryClient();

	const user = $derived(session.user);
	const name = $derived(user?.display_name || user?.username || '');

	let passwordOpen = $state(false);

	async function logout() {
		try {
			await session.logout();
		} catch {
			toast.error('Logout request failed; local session cleared');
		}
		realtime.disconnect();
		queryClient.clear();
		await goto(resolve('/login'));
	}
</script>

{#if user}
	<DropdownMenu.Root>
		<DropdownMenu.Trigger
			class="rounded-full ring-offset-2 ring-offset-background outline-none focus-visible:ring-2 focus-visible:ring-ring"
			aria-label="Account menu"
			data-testid="user-menu"
		>
			<ProjectAvatar seed={user.username} class="size-8" />
		</DropdownMenu.Trigger>
		<DropdownMenu.Content class="min-w-56 rounded-lg" align="end" sideOffset={8}>
			<DropdownMenu.Label class="font-normal">
				<div class="grid text-sm leading-tight">
					<span class="truncate font-medium">{name}</span>
					<span class="truncate text-xs text-muted-foreground">
						{user.email || user.username} · <span class="capitalize">{user.role}</span> ·
						{user.auth_provider === 'oidc' ? 'SSO' : 'local'}
					</span>
				</div>
			</DropdownMenu.Label>
			<DropdownMenu.Separator />
			<DropdownMenu.Sub>
				<DropdownMenu.SubTrigger>
					<SunMoonIcon />
					Theme
				</DropdownMenu.SubTrigger>
				<DropdownMenu.SubContent>
					<DropdownMenu.RadioGroup
						value={userPrefersMode.current}
						onValueChange={(v) => setMode(v as 'light' | 'dark' | 'system')}
					>
						<DropdownMenu.RadioItem value="light">Light</DropdownMenu.RadioItem>
						<DropdownMenu.RadioItem value="dark">Dark</DropdownMenu.RadioItem>
						<DropdownMenu.RadioItem value="system">System</DropdownMenu.RadioItem>
					</DropdownMenu.RadioGroup>
				</DropdownMenu.SubContent>
			</DropdownMenu.Sub>
			<DropdownMenu.Separator />
			{#if user.auth_provider === 'local'}
				<DropdownMenu.Item onclick={() => (passwordOpen = true)}>
					<KeyRoundIcon />
					Change password
				</DropdownMenu.Item>
			{/if}
			<DropdownMenu.Item onclick={logout}>
				<LogOutIcon />
				Log out
			</DropdownMenu.Item>
		</DropdownMenu.Content>
	</DropdownMenu.Root>

	<SetPasswordDialog
		bind:open={passwordOpen}
		own
		onsubmit={async (current, next) => {
			await usersApi.changeOwnPassword({ current_password: current, new_password: next });
			toast.success('Password changed');
		}}
	/>
{/if}

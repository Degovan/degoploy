<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { useQueryClient } from '@tanstack/svelte-query';
	import { setMode, userPrefersMode } from 'mode-watcher';
	import { toast } from 'svelte-sonner';
	import * as Sidebar from '#lib/components/ui/sidebar/index.js';
	import * as DropdownMenu from '#lib/components/ui/dropdown-menu/index.js';
	import * as Avatar from '#lib/components/ui/avatar/index.js';
	import { session } from '#lib/stores/session.svelte.js';
	import { realtime } from '#lib/realtime.svelte.js';
	import ChevronsUpDownIcon from '@lucide/svelte/icons/chevrons-up-down';
	import LogOutIcon from '@lucide/svelte/icons/log-out';
	import SunMoonIcon from '@lucide/svelte/icons/sun-moon';
	import KeyRoundIcon from '@lucide/svelte/icons/key-round';
	import SetPasswordDialog from '#lib/components/set-password-dialog.svelte';
	import { usersApi } from '#lib/api/users.js';

	const sidebar = Sidebar.useSidebar();
	const queryClient = useQueryClient();

	const user = $derived(session.user);
	const name = $derived(user?.display_name || user?.username || '');
	const initials = $derived(
		name
			.split(/[\s._-]+/)
			.filter(Boolean)
			.slice(0, 2)
			.map((p) => p[0]!.toUpperCase())
			.join('') || '?'
	);

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
	<Sidebar.Menu>
		<Sidebar.MenuItem>
			<DropdownMenu.Root>
				<DropdownMenu.Trigger>
					{#snippet child({ props })}
						<Sidebar.MenuButton
							size="lg"
							class="data-[state=open]:bg-sidebar-accent data-[state=open]:text-sidebar-accent-foreground"
							{...props}
						>
							<Avatar.Root class="size-8 rounded-lg">
								<Avatar.Fallback class="rounded-lg">{initials}</Avatar.Fallback>
							</Avatar.Root>
							<div class="grid flex-1 text-left text-sm leading-tight">
								<span class="truncate font-medium">{name}</span>
								<span class="truncate text-xs text-muted-foreground capitalize">{user.role}</span>
							</div>
							<ChevronsUpDownIcon class="ml-auto size-4" />
						</Sidebar.MenuButton>
					{/snippet}
				</DropdownMenu.Trigger>
				<DropdownMenu.Content
					class="w-(--bits-dropdown-menu-anchor-width) min-w-56 rounded-lg"
					side={sidebar.isMobile ? 'bottom' : 'right'}
					align="end"
					sideOffset={4}
				>
					<DropdownMenu.Label class="font-normal">
						<div class="grid text-sm leading-tight">
							<span class="truncate font-medium">{name}</span>
							<span class="truncate text-xs text-muted-foreground">
								{user.email || user.username} · {user.auth_provider === 'oidc' ? 'SSO' : 'local'}
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
		</Sidebar.MenuItem>
	</Sidebar.Menu>

	<SetPasswordDialog
		bind:open={passwordOpen}
		own
		onsubmit={async (current, next) => {
			await usersApi.changeOwnPassword({ current_password: current, new_password: next });
			toast.success('Password changed');
		}}
	/>
{/if}

<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { setMode, userPrefersMode } from 'mode-watcher';
	import { toast } from 'svelte-sonner';
	import { api } from '#lib/api/client.js';
	import type { AuthProviders } from '#lib/api/types.gen.js';
	import ChangePasswordDialog from '#lib/components/change-password-dialog.svelte';
	import { roleInfo } from '#lib/components/role-picker.svelte';
	import UserAvatar from '#lib/components/user-avatar.svelte';
	import * as DropdownMenu from '#lib/components/ui/dropdown-menu/index.js';
	import { session } from '#lib/stores/session.svelte.js';
	import { realtime } from '#lib/realtime.svelte.js';
	import { cn } from '#lib/utils.js';
	import ChevronDownIcon from '@lucide/svelte/icons/chevron-down';
	import FingerprintIcon from '@lucide/svelte/icons/fingerprint-pattern';
	import KeyRoundIcon from '@lucide/svelte/icons/key-round';
	import LockIcon from '@lucide/svelte/icons/lock';
	import LogOutIcon from '@lucide/svelte/icons/log-out';
	import MonitorIcon from '@lucide/svelte/icons/monitor';
	import MoonIcon from '@lucide/svelte/icons/moon';
	import SunIcon from '@lucide/svelte/icons/sun';
	import SunMoonIcon from '@lucide/svelte/icons/sun-moon';

	const queryClient = useQueryClient();
	const user = $derived(session.user);

	const providers = createQuery(() => ({
		queryKey: ['auth', 'providers'],
		queryFn: () => api.get<AuthProviders>('/auth/providers'),
		staleTime: Infinity
	}));
	const ssoName = $derived(providers.data?.oidc.name || 'SSO');

	let open = $state(false);
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

	// Theme is switched inline; the three buttons behave as a radio group.
	const themes = [
		{ value: 'light', label: 'Light', icon: SunIcon },
		{ value: 'dark', label: 'Dark', icon: MoonIcon },
		{ value: 'system', label: 'System', icon: MonitorIcon }
	] as const;
	let themeButtons = $state<HTMLButtonElement[]>([]);
	function onThemeKeydown(e: KeyboardEvent) {
		const step = { ArrowRight: 1, ArrowLeft: -1 }[e.key];
		if (!step) return;
		e.preventDefault();
		e.stopPropagation();
		const i = themes.findIndex((t) => t.value === userPrefersMode.current);
		const next = (i + step + themes.length) % themes.length;
		setMode(themes[next]!.value);
		themeButtons[next]?.focus();
	}

	const item =
		'h-[34px] gap-2.5 rounded-md px-2.5 text-sm [&_svg]:size-[15px] [&_svg]:text-muted-foreground';
	const chip =
		'inline-flex h-[22px] items-center gap-[5px] rounded-[5px] border border-input px-[7px] text-xs';
</script>

{#if user}
	{@const info = roleInfo(user.role)}
	<DropdownMenu.Root bind:open>
		<DropdownMenu.Trigger
			class={cn(
				'flex h-8 items-center gap-2 rounded-full border border-input py-0 pr-[9px] pl-[3px] transition-colors outline-none hover:bg-muted focus-visible:ring-3 focus-visible:ring-ring/50',
				open && 'bg-accent hover:bg-accent'
			)}
			aria-label="Account menu"
			data-testid="user-menu"
		>
			<UserAvatar {user} class="size-6 text-[11px]" />
			<span class="max-w-32 truncate text-[13px] font-medium">{user.username}</span>
			<ChevronDownIcon
				class={cn(
					'size-3.5 transition-transform',
					open ? 'rotate-180 text-muted-foreground' : 'text-muted-foreground/80'
				)}
			/>
		</DropdownMenu.Trigger>
		<DropdownMenu.Content
			align="end"
			sideOffset={8}
			class="w-[288px] rounded-xl border border-input bg-card p-1.5 shadow-[0_16px_40px_rgb(0_0_0/0.8)] ring-0"
		>
			<div class="flex flex-col gap-3 px-2.5 pt-2.5 pb-3">
				<div class="flex items-center gap-3">
					<UserAvatar {user} class="size-9 text-[13px]" />
					<div class="flex min-w-0 flex-col gap-0.5">
						<span class="truncate text-sm font-medium">{user.display_name || user.username}</span>
						<span class="truncate text-[13px] text-muted-foreground/80"
							>{user.email || `@${user.username}`}</span
						>
					</div>
				</div>
				<div class="flex flex-wrap gap-1.5">
					<span class={cn(chip, 'bg-muted font-medium')}>
						<info.icon
							class={cn(
								'size-3',
								user.role === 'admin' ? 'text-foreground' : 'text-muted-foreground'
							)}
						/>{info.label}
					</span>
					<span class={cn(chip, 'text-muted-foreground')}>
						{#if user.auth_provider === 'oidc'}
							<FingerprintIcon class="size-3 text-muted-foreground/80" />{ssoName}
						{:else}
							<KeyRoundIcon class="size-3 text-muted-foreground/80" />Password
						{/if}
					</span>
				</div>
			</div>

			<DropdownMenu.Separator class="mx-1 my-1" />

			<div class="flex h-10 items-center gap-2.5 pr-1 pl-2.5">
				<SunMoonIcon class="size-[15px] text-muted-foreground" />
				<span id="theme-label" class="flex-1 text-sm">Theme</span>
				<div
					class="flex gap-0.5 rounded-lg border bg-background p-0.5"
					role="radiogroup"
					aria-labelledby="theme-label"
				>
					{#each themes as t, i (t.value)}
						{@const on = userPrefersMode.current === t.value}
						<button
							bind:this={themeButtons[i]}
							type="button"
							role="radio"
							aria-checked={on}
							aria-label={t.label}
							title={t.label}
							tabindex={on ? 0 : -1}
							class={cn(
								'flex h-[26px] w-[30px] items-center justify-center rounded-md border transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring/50 [&_svg]:size-3.5',
								on
									? 'border-input bg-accent text-foreground'
									: 'border-transparent text-muted-foreground/80 hover:text-foreground'
							)}
							onclick={() => setMode(t.value)}
							onkeydown={onThemeKeydown}
						>
							<t.icon />
						</button>
					{/each}
				</div>
			</div>

			<DropdownMenu.Separator class="mx-1 my-1" />

			{#if user.auth_provider === 'local'}
				<DropdownMenu.Item class={item} onSelect={() => (passwordOpen = true)}>
					<KeyRoundIcon />
					Change password
				</DropdownMenu.Item>
			{:else}
				<div
					class="flex h-[34px] items-center gap-2.5 px-2.5 text-[13px] text-muted-foreground/80"
					aria-disabled="true"
				>
					<LockIcon class="size-[15px]" />
					Password managed by {ssoName}
				</div>
			{/if}
			<DropdownMenu.Item class={item} onSelect={logout}>
				<LogOutIcon />
				Log out
			</DropdownMenu.Item>
		</DropdownMenu.Content>
	</DropdownMenu.Root>

	<ChangePasswordDialog bind:open={passwordOpen} />
{/if}

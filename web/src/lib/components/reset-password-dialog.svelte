<script lang="ts">
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { toast } from 'svelte-sonner';
	import { ApiError, errorMessage } from '#lib/api/client.js';
	import { userKeys, usersApi } from '#lib/api/users.js';
	import type { User } from '#lib/api/types.gen.js';
	import { copyText } from '#lib/clipboard.js';
	import PasswordStrength from '#lib/components/password-strength.svelte';
	import { roleInfo } from '#lib/components/role-picker.svelte';
	import UserAvatar from '#lib/components/user-avatar.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import * as Dialog from '#lib/components/ui/dialog/index.js';
	import { generatePassword } from '#lib/password.js';
	import { cn } from '#lib/utils.js';
	import CheckIcon from '@lucide/svelte/icons/check';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import CopyIcon from '@lucide/svelte/icons/copy';
	import EyeIcon from '@lucide/svelte/icons/eye';
	import EyeOffIcon from '@lucide/svelte/icons/eye-off';
	import KeyRoundIcon from '@lucide/svelte/icons/key-round';
	import LoaderIcon from '@lucide/svelte/icons/loader-circle';
	import LogOutIcon from '@lucide/svelte/icons/log-out';
	import WandIcon from '@lucide/svelte/icons/wand-sparkles';
	import XIcon from '@lucide/svelte/icons/x';

	// An admin sets a new password for a local user, optionally making them
	// choose their own at next sign-in.
	let {
		open = $bindable(false),
		user,
		isSelf = false
	}: { open?: boolean; user: User | null; isSelf?: boolean } = $props();

	const queryClient = useQueryClient();

	let password = $state('');
	let visible = $state(false);
	let requireChange = $state(true);
	let error = $state('');
	let saving = $state(false);
	let copied = $state(false);
	let field = $state<HTMLInputElement | null>(null);

	// The sessions the reset will end, fresh each time the dialog opens.
	const sessions = createQuery(() => ({
		queryKey: [...userKeys.all, user?.id, 'sessions'],
		queryFn: () => usersApi.sessions(user!.id),
		enabled: open && !!user,
		staleTime: 0
	}));

	$effect(() => {
		if (!open) return;
		password = '';
		visible = false;
		requireChange = true;
		error = '';
		copied = false;
	});

	const name = $derived(user ? (user.display_name || user.username).split(' ')[0] : '');
	const active = $derived(sessions.data?.active ?? null);
	const sessionsNote = $derived.by(() => {
		if (active === null) return '';
		const n = `${active} active ${active === 1 ? 'session' : 'sessions'}`;
		if (isSelf) {
			return active
				? `Signs you out of ${active} other ${active === 1 ? 'session' : 'sessions'}; this one stays signed in.`
				: 'You have no other active sessions.';
		}
		return active
			? `Signs ${name} out of ${n} right away.`
			: `${name} has no active sessions right now.`;
	});

	function generate() {
		password = generatePassword();
		error = '';
		field?.focus();
	}

	async function copy() {
		if (!password) return;
		if (!(await copyText(password))) {
			toast.error('Could not copy to the clipboard');
			return;
		}
		copied = true;
		setTimeout(() => (copied = false), 1500);
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		if (!user) return;
		error = '';
		if (password.length < 10) {
			error = 'The password must be at least 10 characters.';
			field?.focus();
			return;
		}
		saving = true;
		try {
			const mustChange = requireChange && !isSelf;
			await usersApi.setPassword(user.id, { password, require_change: mustChange });
			await queryClient.invalidateQueries({ queryKey: userKeys.all });
			toast.success(`Password set for ${user.username}`, {
				description: mustChange ? 'They will choose their own at next sign-in.' : undefined
			});
			open = false;
		} catch (err) {
			const msg = errorMessage(err);
			error = err instanceof ApiError && err.field === 'password' ? `The ${msg}.` : msg;
		} finally {
			saving = false;
		}
	}

	const iconBtn =
		'flex size-[30px] shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none disabled:pointer-events-none disabled:opacity-40 [&_svg]:size-[15px]';
</script>

<Dialog.Root bind:open>
	<Dialog.Content
		showCloseButton={false}
		class="flex max-h-[calc(100svh-2rem)] flex-col gap-0 overflow-hidden rounded-xl border border-input bg-card p-0 shadow-[0_24px_64px_rgb(0_0_0/0.8)] ring-0 sm:max-w-[480px]"
		onOpenAutoFocus={(e) => {
			e.preventDefault();
			requestAnimationFrame(() => field?.focus());
		}}
	>
		{#if user}
			{@const info = roleInfo(user.role)}
			<form class="contents" onsubmit={submit} novalidate>
				<div
					class="flex min-h-0 flex-1 [scrollbar-width:thin] flex-col gap-5 overflow-y-auto overscroll-contain p-6"
				>
					<div class="flex items-start justify-between gap-4">
						<Dialog.Header class="gap-1.5">
							<Dialog.Title class="text-xl font-semibold tracking-[-0.4px]"
								>Reset password</Dialog.Title
							>
							<Dialog.Description class="text-sm text-muted-foreground">
								Set a new password, then share it with them through a secure channel.
							</Dialog.Description>
						</Dialog.Header>
						<Dialog.Close class={cn(iconBtn, '-mt-1 -mr-1')} aria-label="Close">
							<XIcon />
						</Dialog.Close>
					</div>

					<div class="flex items-center gap-3 rounded-lg border bg-background px-3 py-2.5">
						<UserAvatar {user} />
						<div class="flex min-w-0 flex-1 flex-col gap-[3px]">
							<span class="truncate text-sm font-medium">{user.display_name || user.username}</span>
							<span class="flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground/80">
								<span class="truncate font-mono">@{user.username}</span>
								<span class="text-muted-foreground/40">·</span>
								<span class="shrink-0">Password sign-in</span>
							</span>
						</div>
						<span
							class="inline-flex h-[22px] shrink-0 items-center gap-[5px] rounded-[5px] border border-input bg-muted px-[7px] text-xs font-medium"
						>
							<info.icon
								class={cn(
									'size-3',
									user.role === 'admin' ? 'text-foreground' : 'text-muted-foreground'
								)}
							/>
							{info.label}
						</span>
					</div>

					<div class="flex flex-col gap-2">
						<label for="reset-password" class="text-[13px] font-medium text-muted-foreground"
							>New password</label
						>
						<div
							class={cn(
								'flex h-10 items-center gap-0.5 rounded-md border border-input bg-background pr-1 pl-3 transition-[color,box-shadow] focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/40',
								error && 'border-destructive focus-within:border-destructive'
							)}
						>
							<input
								bind:this={field}
								bind:value={password}
								id="reset-password"
								type={visible ? 'text' : 'password'}
								autocomplete="new-password"
								spellcheck={false}
								class="h-full min-w-0 flex-1 bg-transparent font-mono text-[13px] tracking-wide outline-none placeholder:font-sans placeholder:tracking-normal placeholder:text-muted-foreground/70"
								placeholder="Type or generate one"
								aria-invalid={!!error}
								oninput={() => (error = '')}
							/>
							<button
								type="button"
								class={iconBtn}
								aria-label={visible ? 'Hide password' : 'Show password'}
								aria-pressed={visible}
								onclick={() => (visible = !visible)}
							>
								{#if visible}<EyeOffIcon />{:else}<EyeIcon />{/if}
							</button>
							<button
								type="button"
								class={iconBtn}
								aria-label="Copy password"
								disabled={!password}
								onclick={copy}
							>
								{#if copied}<CheckIcon />{:else}<CopyIcon />{/if}
							</button>
							<Button
								type="button"
								variant="outline"
								class="ml-0.5 h-[30px] gap-[7px] px-2.5 text-xs [&_svg]:size-[13px] [&_svg]:text-muted-foreground"
								onclick={generate}
							>
								<WandIcon />
								Generate
							</Button>
						</div>
						{#if error}
							<p class="text-xs/normal text-destructive" role="alert">{error}</p>
						{:else}
							<PasswordStrength {password} />
						{/if}
					</div>

					{#if !isSelf}
						<label class="flex cursor-pointer items-start gap-3">
							<span class="relative mt-px flex size-4 shrink-0">
								<input
									type="checkbox"
									bind:checked={requireChange}
									class="peer size-4 cursor-pointer appearance-none rounded border border-input bg-background transition-colors outline-none checked:border-foreground checked:bg-foreground focus-visible:ring-3 focus-visible:ring-ring/50"
								/>
								<CheckIcon
									class="pointer-events-none absolute inset-0 m-auto hidden size-3 text-background peer-checked:block"
									strokeWidth={3}
								/>
							</span>
							<span class="flex flex-col gap-[3px]">
								<span class="text-[13px] font-medium">Require a new password on next sign-in</span>
								<span class="text-xs text-muted-foreground/80"
									>They'll pick their own password right after signing in.</span
								>
							</span>
						</label>
					{/if}

					{#if sessions.isError}
						<p class="flex items-center gap-2.5 text-xs text-muted-foreground/80">
							<CircleAlertIcon class="size-3.5" />
							Could not count active sessions; all of them still end.
						</p>
					{:else if sessionsNote}
						<p
							class={cn(
								'flex items-center gap-2.5 rounded-lg border px-3 py-2.5 text-xs text-muted-foreground',
								active ? 'border-amber-400/20 bg-amber-400/[0.06]' : 'bg-background'
							)}
						>
							<LogOutIcon
								class={cn(
									'size-[15px] shrink-0',
									active ? 'text-amber-400' : 'text-muted-foreground/80'
								)}
							/>
							{sessionsNote}
						</p>
					{/if}
				</div>

				<div
					class="flex shrink-0 items-center justify-between gap-3 border-t bg-background px-6 py-4"
				>
					<Button
						type="button"
						variant="outline"
						class="h-9 px-3 text-[13px]"
						onclick={() => (open = false)}
						disabled={saving}
					>
						Cancel
					</Button>
					<Button type="submit" class="h-9 px-3.5 text-[13px]" disabled={saving}>
						{#if saving}<LoaderIcon class="animate-spin" />{:else}<KeyRoundIcon />{/if}
						Set password
					</Button>
				</div>
			</form>
		{/if}
	</Dialog.Content>
</Dialog.Root>

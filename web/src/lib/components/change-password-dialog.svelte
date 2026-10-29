<script lang="ts">
	import { createQuery } from '@tanstack/svelte-query';
	import { toast } from 'svelte-sonner';
	import { api, ApiError, errorMessage } from '#lib/api/client.js';
	import type { UserSessions } from '#lib/api/types.gen.js';
	import { usersApi } from '#lib/api/users.js';
	import PasswordStrength from '#lib/components/password-strength.svelte';
	import UserAvatar from '#lib/components/user-avatar.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import * as Dialog from '#lib/components/ui/dialog/index.js';
	import { session } from '#lib/stores/session.svelte.js';
	import { cn } from '#lib/utils.js';
	import CheckIcon from '@lucide/svelte/icons/check';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import EyeIcon from '@lucide/svelte/icons/eye';
	import EyeOffIcon from '@lucide/svelte/icons/eye-off';
	import KeyRoundIcon from '@lucide/svelte/icons/key-round';
	import LoaderIcon from '@lucide/svelte/icons/loader-circle';
	import LogOutIcon from '@lucide/svelte/icons/log-out';
	import XIcon from '@lucide/svelte/icons/x';

	// Changing your own password; it needs the current one and signs out your
	// other sessions.
	let { open = $bindable(false) }: { open?: boolean } = $props();

	type Field = 'current' | 'next' | 'confirm';
	let values = $state<Record<Field, string>>({ current: '', next: '', confirm: '' });
	let visible = $state<Record<Field, boolean>>({ current: false, next: false, confirm: false });
	let errors = $state<Partial<Record<Field | 'form', string>>>({});
	let saving = $state(false);
	let inputs = $state<Partial<Record<Field, HTMLInputElement>>>({});

	const sessions = createQuery(() => ({
		queryKey: ['me', 'sessions'],
		queryFn: () => api.get<UserSessions>('/me/sessions'),
		enabled: open,
		staleTime: 0
	}));
	const others = $derived(sessions.data?.active ?? null);

	$effect(() => {
		if (!open) return;
		values = { current: '', next: '', confirm: '' };
		visible = { current: false, next: false, confirm: false };
		errors = {};
	});

	const matches = $derived(values.confirm !== '' && values.confirm === values.next);

	function sentence(msg: string) {
		return msg.charAt(0).toUpperCase() + msg.slice(1) + (msg.endsWith('.') ? '' : '.');
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		errors = {};
		if (!values.current) errors.current = 'Enter your current password.';
		if (values.next.length < 10) errors.next = 'Use at least 10 characters.';
		else if (values.next === values.current)
			errors.next = 'Pick a password different from the current one.';
		if (!errors.next && values.confirm !== values.next)
			errors.confirm = 'The passwords do not match.';
		const first = (['current', 'next', 'confirm'] as Field[]).find((f) => errors[f]);
		if (first) {
			inputs[first]?.focus();
			return;
		}
		saving = true;
		try {
			await usersApi.changeOwnPassword({
				current_password: values.current,
				new_password: values.next
			});
			toast.success('Password changed', {
				description: others
					? `Signed out of ${others} other ${others === 1 ? 'session' : 'sessions'}.`
					: undefined
			});
			open = false;
		} catch (err) {
			if (err instanceof ApiError && err.field === 'current_password') {
				errors.current = sentence(`The current password ${err.message}`);
				inputs.current?.focus();
			} else if (err instanceof ApiError && err.field === 'new_password') {
				errors.next = sentence(err.message);
				inputs.next?.focus();
			} else {
				errors.form = errorMessage(err);
			}
		} finally {
			saving = false;
		}
	}

	const label = 'text-[13px] font-medium text-muted-foreground';
</script>

{#snippet secret(f: Field, id: string, autocomplete: 'current-password' | 'new-password')}
	<div
		class={cn(
			'flex h-10 items-center gap-0.5 rounded-md border border-input bg-background pr-1 pl-3 transition-[color,box-shadow] focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/40',
			errors[f] && 'border-destructive focus-within:border-destructive'
		)}
	>
		<input
			bind:this={inputs[f]}
			bind:value={values[f]}
			{id}
			type={visible[f] ? 'text' : 'password'}
			{autocomplete}
			spellcheck={false}
			aria-invalid={!!errors[f]}
			class="h-full min-w-0 flex-1 bg-transparent font-mono text-[13px] tracking-wide outline-none"
			oninput={() => delete errors[f]}
		/>
		<button
			type="button"
			class="flex size-[30px] shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none [&_svg]:size-[15px]"
			aria-label={visible[f] ? 'Hide password' : 'Show password'}
			aria-pressed={visible[f]}
			onclick={() => (visible[f] = !visible[f])}
		>
			{#if visible[f]}<EyeOffIcon />{:else}<EyeIcon />{/if}
		</button>
	</div>
{/snippet}

{#snippet fieldError(f: Field)}
	{#if errors[f]}<p class="text-xs/normal text-destructive" role="alert">{errors[f]}</p>{/if}
{/snippet}

<Dialog.Root bind:open>
	<Dialog.Content
		showCloseButton={false}
		class="flex max-h-[calc(100svh-2rem)] flex-col gap-0 overflow-hidden rounded-xl border border-input bg-card p-0 shadow-[0_24px_64px_rgb(0_0_0/0.8)] ring-0 sm:max-w-[480px]"
		onOpenAutoFocus={(e) => {
			e.preventDefault();
			requestAnimationFrame(() => inputs.current?.focus());
		}}
	>
		<form class="contents" onsubmit={submit} novalidate>
			<div
				class="flex min-h-0 flex-1 [scrollbar-width:thin] flex-col gap-5 overflow-y-auto overscroll-contain p-6"
			>
				<div class="flex items-start justify-between gap-4">
					<Dialog.Header class="gap-1.5">
						<Dialog.Title class="text-xl font-semibold tracking-[-0.4px]"
							>Change password</Dialog.Title
						>
						<Dialog.Description class="text-sm text-muted-foreground">
							Pick a new password for signing in to degoploy.
						</Dialog.Description>
					</Dialog.Header>
					<Dialog.Close
						class="-mt-1 -mr-1 flex size-[30px] shrink-0 items-center justify-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none [&_svg]:size-[15px]"
						aria-label="Close"
					>
						<XIcon />
					</Dialog.Close>
				</div>

				{#if session.user}
					{@const u = session.user}
					<div class="flex items-center gap-3 rounded-lg border bg-background px-3 py-2.5">
						<UserAvatar user={u} />
						<div class="flex min-w-0 flex-col gap-[3px]">
							<span class="truncate text-sm font-medium">{u.display_name || u.username}</span>
							<span class="flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground/80">
								<span class="truncate font-mono">@{u.username}</span>
								<span class="text-muted-foreground/40">·</span>
								<span class="shrink-0">Password sign-in</span>
							</span>
						</div>
					</div>
				{/if}

				{#if errors.form}
					<p
						class="flex items-start gap-2 rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-[13px] text-destructive"
						role="alert"
					>
						<CircleAlertIcon class="mt-0.5 size-3.5 shrink-0" />
						{errors.form}
					</p>
				{/if}

				<div class="flex flex-col gap-2">
					<label for="cp-current" class={label}>Current password</label>
					{@render secret('current', 'cp-current', 'current-password')}
					{#if errors.current}
						{@render fieldError('current')}
					{:else}
						<p class="text-xs/normal text-muted-foreground/80">
							Forgot it? An admin can reset it from the Users page.
						</p>
					{/if}
				</div>

				<div class="flex flex-col gap-2">
					<label for="cp-new" class={label}>New password</label>
					{@render secret('next', 'cp-new', 'new-password')}
					{#if errors.next}
						{@render fieldError('next')}
					{:else}
						<PasswordStrength password={values.next} />
					{/if}
				</div>

				<div class="flex flex-col gap-2">
					<label for="cp-confirm" class={label}>Confirm new password</label>
					{@render secret('confirm', 'cp-confirm', 'new-password')}
					{#if errors.confirm}
						{@render fieldError('confirm')}
					{:else if values.confirm}
						<p
							class={cn(
								'flex items-center gap-1.5 text-xs',
								matches ? 'text-emerald-400' : 'text-muted-foreground/80'
							)}
						>
							{#if matches}<CheckIcon class="size-3" />Passwords match{:else}Doesn't match yet{/if}
						</p>
					{/if}
				</div>

				{#if others !== null}
					<p
						class={cn(
							'flex items-center gap-2.5 rounded-lg border px-3 py-2.5 text-xs text-muted-foreground',
							others ? 'border-amber-400/20 bg-amber-400/[0.06]' : 'bg-background'
						)}
					>
						<LogOutIcon
							class={cn(
								'size-[15px] shrink-0',
								others ? 'text-amber-400' : 'text-muted-foreground/80'
							)}
						/>
						{others
							? `Signs you out of ${others} other ${others === 1 ? 'session' : 'sessions'}. This one stays signed in.`
							: 'You are not signed in anywhere else. This session stays signed in.'}
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
					Change password
				</Button>
			</div>
		</form>
	</Dialog.Content>
</Dialog.Root>

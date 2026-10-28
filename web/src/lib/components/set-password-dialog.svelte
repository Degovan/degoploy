<script lang="ts">
	import { ApiError, errorMessage } from '#lib/api/client.js';
	import PasswordField from '#lib/components/password-field.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import * as Dialog from '#lib/components/ui/dialog/index.js';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import KeyRoundIcon from '@lucide/svelte/icons/key-round';
	import LoaderIcon from '@lucide/svelte/icons/loader-circle';

	/**
	 * own = true asks for the current password (self-service change);
	 * otherwise an admin sets a new password for `username`.
	 */
	let {
		open = $bindable(false),
		own = false,
		username = '',
		onsubmit
	}: {
		open?: boolean;
		own?: boolean;
		username?: string;
		onsubmit: (current: string, next: string) => Promise<void>;
	} = $props();

	let current = $state('');
	let next = $state('');
	let confirm = $state('');
	let errors = $state<Record<string, string>>({});
	let saving = $state(false);

	$effect(() => {
		if (!open) return;
		current = next = confirm = '';
		errors = {};
	});

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		errors = {};
		if (own && next !== confirm) {
			errors = { confirm: 'Passwords do not match' };
			return;
		}
		saving = true;
		try {
			await onsubmit(current, next);
			open = false;
		} catch (err) {
			if (err instanceof ApiError && err.field) {
				errors = { [err.field === 'password' ? 'new_password' : err.field]: err.message };
			} else {
				errors = { form: errorMessage(err) };
			}
		} finally {
			saving = false;
		}
	}

	const label = 'text-[13px] leading-[17px] text-muted-foreground';
</script>

{#snippet fieldError(name: string)}
	{#if errors[name]}<p class="text-xs/normal text-destructive">{errors[name]}</p>{/if}
{/snippet}

<Dialog.Root bind:open>
	<Dialog.Content
		showCloseButton={false}
		class="flex max-h-[calc(100svh-2rem)] flex-col gap-0 overflow-hidden rounded-xl border border-input bg-card p-0 shadow-[0_24px_64px_rgb(0_0_0/0.8)] ring-0 sm:max-w-[460px]"
	>
		<form class="contents" onsubmit={submit}>
			<div
				class="flex min-h-0 flex-1 [scrollbar-width:thin] flex-col gap-5 overflow-y-auto overscroll-contain p-6"
			>
				<Dialog.Header class="gap-1.5">
					<Dialog.Title class="text-xl font-semibold tracking-[-0.4px]">
						{own ? 'Change password' : `Reset password for ${username}`}
					</Dialog.Title>
					<Dialog.Description class="text-sm text-muted-foreground">
						{own
							? 'Your other sessions are signed out; this one stays signed in.'
							: 'All of their sessions are signed out. Share the new password securely.'}
					</Dialog.Description>
				</Dialog.Header>
				{#if errors.form}
					<p
						class="flex items-start gap-2 rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-[13px] text-destructive"
						role="alert"
					>
						<CircleAlertIcon class="mt-0.5 size-3.5 shrink-0" />
						{errors.form}
					</p>
				{/if}
				{#if own}
					<div class="flex flex-col gap-2">
						<Label for="pw-current" class={label}>Current password</Label>
						<PasswordField
							id="pw-current"
							bind:value={current}
							autocomplete="current-password"
							generate={false}
							invalid={!!errors.current_password}
						/>
						{@render fieldError('current_password')}
					</div>
				{/if}
				<div class="flex flex-col gap-2">
					<Label for="pw-new" class={label}>New password</Label>
					<PasswordField
						id="pw-new"
						bind:value={next}
						generate={!own}
						invalid={!!errors.new_password}
					/>
					{#if errors.new_password}
						{@render fieldError('new_password')}
					{:else}
						<p class="text-xs/normal text-muted-foreground/80">At least 10 characters.</p>
					{/if}
				</div>
				{#if own}
					<div class="flex flex-col gap-2">
						<Label for="pw-confirm" class={label}>Confirm new password</Label>
						<PasswordField
							id="pw-confirm"
							bind:value={confirm}
							generate={false}
							invalid={!!errors.confirm}
						/>
						{@render fieldError('confirm')}
					</div>
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
					{own ? 'Change password' : 'Set password'}
				</Button>
			</div>
		</form>
	</Dialog.Content>
</Dialog.Root>

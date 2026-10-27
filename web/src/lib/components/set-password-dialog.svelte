<script lang="ts">
	import { ApiError, errorMessage } from '#lib/api/client.js';
	import PasswordField from '#lib/components/password-field.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import * as Dialog from '#lib/components/ui/dialog/index.js';
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
</script>

{#snippet fieldError(name: string)}
	{#if errors[name]}<p class="text-sm text-destructive">{errors[name]}</p>{/if}
{/snippet}

<Dialog.Root bind:open>
	<Dialog.Content class="sm:max-w-md">
		<Dialog.Header>
			<Dialog.Title>{own ? 'Change password' : `Reset password for ${username}`}</Dialog.Title>
			<Dialog.Description>
				{own
					? 'Your other sessions are signed out; this one stays signed in.'
					: 'All of their sessions are signed out. Share the new password securely.'}
			</Dialog.Description>
		</Dialog.Header>
		<form id="password-form" class="flex flex-col gap-4" onsubmit={submit}>
			{@render fieldError('form')}
			{#if own}
				<div class="flex flex-col gap-2">
					<Label for="pw-current">Current password</Label>
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
				<Label for="pw-new">New password</Label>
				<PasswordField
					id="pw-new"
					bind:value={next}
					generate={!own}
					invalid={!!errors.new_password}
				/>
				{@render fieldError('new_password')}
			</div>
			{#if own}
				<div class="flex flex-col gap-2">
					<Label for="pw-confirm">Confirm new password</Label>
					<PasswordField
						id="pw-confirm"
						bind:value={confirm}
						generate={false}
						invalid={!!errors.confirm}
					/>
					{@render fieldError('confirm')}
				</div>
			{/if}
		</form>
		<Dialog.Footer>
			<Button variant="ghost" onclick={() => (open = false)} disabled={saving}>Cancel</Button>
			<Button type="submit" form="password-form" disabled={saving}>
				{#if saving}<LoaderIcon class="animate-spin" />{/if}
				{own ? 'Change password' : 'Set password'}
			</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>

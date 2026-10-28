<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { useQueryClient } from '@tanstack/svelte-query';
	import { api, ApiError, errorMessage } from '#lib/api/client.js';
	import { usersApi } from '#lib/api/users.js';
	import type { Me } from '#lib/api/types.gen.js';
	import Logo from '#lib/components/logo.svelte';
	import PasswordField from '#lib/components/password-field.svelte';
	import PasswordStrength from '#lib/components/password-strength.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import { session } from '#lib/stores/session.svelte.js';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import LoaderIcon from '@lucide/svelte/icons/loader-circle';

	// Shown instead of the app after an admin reset the password with
	// "require a new password": the API refuses everything else until then.
	const queryClient = useQueryClient();

	let current = $state('');
	let next = $state('');
	let confirm = $state('');
	let errors = $state<Record<string, string>>({});
	let saving = $state(false);

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		errors = {};
		if (next !== confirm) {
			errors = { confirm: 'The passwords do not match.' };
			return;
		}
		if (next === current) {
			errors = { new_password: 'Pick a password different from the one you were given.' };
			return;
		}
		saving = true;
		try {
			await usersApi.changeOwnPassword({ current_password: current, new_password: next });
			session.set(await api.get<Me>('/me'));
		} catch (err) {
			if (err instanceof ApiError && err.field) {
				const msg = err.field === 'current_password' ? `The password ${err.message}` : err.message;
				errors = { [err.field]: `${msg.charAt(0).toUpperCase()}${msg.slice(1)}.` };
			} else {
				errors = { form: errorMessage(err) };
			}
		} finally {
			saving = false;
		}
	}

	async function logout() {
		await session.logout().catch(() => {});
		queryClient.clear();
		await goto(resolve('/login'));
	}

	const label = 'text-[13px] leading-[17px] text-muted-foreground';
</script>

<svelte:head>
	<title>Choose a new password · degoploy</title>
</svelte:head>

{#snippet fieldError(key: string)}
	{#if errors[key]}<p class="text-xs/normal text-destructive">{errors[key]}</p>{/if}
{/snippet}

<div class="flex min-h-svh items-center justify-center p-4">
	<div class="flex w-full max-w-[420px] flex-col gap-6">
		<div class="flex items-center justify-center gap-2">
			<Logo class="size-8" />
			<span class="text-xl font-semibold tracking-tight">degoploy</span>
		</div>

		<form class="flex flex-col gap-5 rounded-xl border bg-card p-6" onsubmit={submit} novalidate>
			<div class="flex flex-col gap-1.5">
				<h1 class="text-xl font-semibold tracking-[-0.4px]">Choose a new password</h1>
				<p class="text-sm text-muted-foreground">
					An admin reset your password. Pick your own to continue as
					<span class="text-foreground">@{session.user?.username}</span>.
				</p>
			</div>

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
				<Label for="gate-current" class={label}>Password you were given</Label>
				<PasswordField
					id="gate-current"
					bind:value={current}
					autocomplete="current-password"
					generate={false}
					invalid={!!errors.current_password}
				/>
				{@render fieldError('current_password')}
			</div>
			<div class="flex flex-col gap-2">
				<Label for="gate-new" class={label}>New password</Label>
				<PasswordField
					id="gate-new"
					bind:value={next}
					generate={false}
					invalid={!!errors.new_password}
				/>
				{#if errors.new_password}
					{@render fieldError('new_password')}
				{:else}
					<PasswordStrength password={next} />
				{/if}
			</div>
			<div class="flex flex-col gap-2">
				<Label for="gate-confirm" class={label}>Confirm new password</Label>
				<PasswordField
					id="gate-confirm"
					bind:value={confirm}
					generate={false}
					invalid={!!errors.confirm}
				/>
				{@render fieldError('confirm')}
			</div>

			<Button type="submit" class="h-10 text-[13px]" disabled={saving}>
				{#if saving}<LoaderIcon class="animate-spin" />{/if}
				Save and continue
			</Button>
			<button
				type="button"
				class="self-center text-[13px] text-muted-foreground underline-offset-2 hover:text-foreground hover:underline"
				onclick={logout}
			>
				Sign out
			</button>
		</form>
	</div>
</div>

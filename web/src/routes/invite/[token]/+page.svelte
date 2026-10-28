<script lang="ts">
	import { createQuery } from '@tanstack/svelte-query';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { ApiError, errorMessage } from '#lib/api/client.js';
	import { invitesApi } from '#lib/api/users.js';
	import Logo from '#lib/components/logo.svelte';
	import PasswordField from '#lib/components/password-field.svelte';
	import { roleInfo } from '#lib/components/role-picker.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import { session } from '#lib/stores/session.svelte.js';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import LinkIcon from '@lucide/svelte/icons/link-2-off';
	import LoaderIcon from '@lucide/svelte/icons/loader-circle';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const invite = createQuery(() => ({
		queryKey: ['invite', data.token],
		queryFn: () => invitesApi.preview(data.token),
		retry: false
	}));

	let username = $state('');
	let displayName = $state('');
	let password = $state('');
	let errors = $state<Record<string, string>>({});
	let formError = $state('');
	let saving = $state(false);

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		errors = {};
		formError = '';
		saving = true;
		try {
			session.set(
				await invitesApi.accept(data.token, {
					username: username.trim(),
					display_name: displayName.trim(),
					password
				})
			);
			await goto(resolve('/(app)'), { replaceState: true });
		} catch (err) {
			if (err instanceof ApiError && err.field) errors = { [err.field]: err.message };
			else formError = errorMessage(err);
		} finally {
			saving = false;
		}
	}

	const label = 'text-[13px] leading-[17px] text-muted-foreground';
	const field = 'h-10 px-3 bg-background dark:bg-background text-sm';
</script>

<svelte:head>
	<title>Join degoploy</title>
</svelte:head>

<!-- Server messages may be fragments such as "is already taken"; `name` completes them. -->
{#snippet fieldError(key: string, name = '')}
	{#if errors[key]}
		{@const msg = name ? `${name} ${errors[key]}` : errors[key]}
		<p class="text-xs/normal text-destructive">{msg.charAt(0).toUpperCase() + msg.slice(1)}.</p>
	{/if}
{/snippet}

<div class="flex min-h-svh items-center justify-center p-4">
	<div class="flex w-full max-w-[420px] flex-col gap-6">
		<div class="flex items-center justify-center gap-2">
			<Logo class="size-8" />
			<span class="text-xl font-semibold tracking-tight">degoploy</span>
		</div>

		<div class="overflow-hidden rounded-xl border bg-card">
			{#if invite.isPending}
				<div class="flex justify-center py-16 text-muted-foreground">
					<LoaderIcon class="size-5 animate-spin" />
				</div>
			{:else if invite.isError}
				<div class="flex flex-col items-center gap-3 px-6 py-12 text-center">
					<span
						class="flex size-10 items-center justify-center rounded-full border bg-muted text-muted-foreground"
					>
						<LinkIcon class="size-4" />
					</span>
					<h1 class="text-lg font-semibold">
						{invite.error instanceof ApiError && invite.error.status === 410
							? 'This invite has expired'
							: 'This invite link does not work'}
					</h1>
					<p class="max-w-xs text-sm text-muted-foreground">
						{invite.error.message.charAt(0).toUpperCase() + invite.error.message.slice(1)}.
					</p>
					<Button href={resolve('/login')} variant="outline" class="mt-2 h-9 px-3.5 text-[13px]"
						>Go to sign in</Button
					>
				</div>
			{:else}
				{@const info = roleInfo(invite.data.role)}
				<form class="flex flex-col gap-5 p-6" onsubmit={submit} novalidate>
					<div class="flex flex-col gap-1.5">
						<h1 class="text-xl font-semibold tracking-[-0.4px]">Create your account</h1>
						<p class="text-sm text-muted-foreground">
							{invite.data.invited_by
								? `@${invite.data.invited_by} invited you`
								: 'You were invited'}
							to join as
							<span class="inline-flex items-center gap-1 font-medium text-foreground"
								><info.icon class="size-3.5" />{info.label}</span
							>.
						</p>
					</div>

					{#if session.me}
						<p class="rounded-md border bg-background px-3 py-2 text-[13px] text-muted-foreground">
							You are signed in as <span class="text-foreground">@{session.me.user.username}</span>.
							Creating this account signs you out of it.
						</p>
					{/if}

					{#if formError}
						<p
							class="flex items-start gap-2 rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-[13px] text-destructive"
							role="alert"
						>
							<CircleAlertIcon class="mt-0.5 size-3.5 shrink-0" />
							{formError}
						</p>
					{/if}

					<div class="flex flex-col gap-2">
						<Label for="invite-email" class={label}>Email</Label>
						<Input
							id="invite-email"
							value={invite.data.email}
							readonly
							class="{field} text-muted-foreground"
						/>
					</div>
					<div class="flex flex-col gap-2">
						<Label for="invite-username" class={label}>Username</Label>
						<Input
							id="invite-username"
							bind:value={username}
							required
							autocomplete="username"
							spellcheck={false}
							class={field}
							aria-invalid={!!errors.username}
						/>
						{#if errors.username}
							{@render fieldError('username', 'This username')}
						{:else}
							<p class="text-xs/normal text-muted-foreground/80">
								You sign in with this. Letters, digits, dots, dashes or underscores.
							</p>
						{/if}
					</div>
					<div class="flex flex-col gap-2">
						<Label for="invite-display" class={label}>Display name (optional)</Label>
						<Input
							id="invite-display"
							bind:value={displayName}
							autocomplete="name"
							placeholder={username}
							class={field}
						/>
					</div>
					<div class="flex flex-col gap-2">
						<Label for="invite-password" class={label}>Password</Label>
						<PasswordField
							id="invite-password"
							bind:value={password}
							generate={false}
							invalid={!!errors.password}
						/>
						{#if errors.password}
							{@render fieldError('password')}
						{:else}
							<p class="text-xs/normal text-muted-foreground/80">At least 10 characters.</p>
						{/if}
					</div>

					<Button type="submit" class="h-10 text-[13px]" disabled={saving}>
						{#if saving}<LoaderIcon class="animate-spin" />{/if}
						Create account
					</Button>
				</form>
			{/if}
		</div>
	</div>
</div>

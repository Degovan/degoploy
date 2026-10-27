<script lang="ts">
	import { createQuery } from '@tanstack/svelte-query';
	import { goto } from '$app/navigation';
	import { api, ApiError } from '#lib/api/client.js';
	import type { AuthProviders, LoginRequest, Me } from '#lib/api/types.gen.js';
	import { session } from '#lib/stores/session.svelte.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import * as Card from '#lib/components/ui/card/index.js';
	import * as Alert from '#lib/components/ui/alert/index.js';
	import Logo from '#lib/components/logo.svelte';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import KeyRoundIcon from '@lucide/svelte/icons/key-round';
	import LoaderIcon from '@lucide/svelte/icons/loader-circle';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const providers = createQuery(() => ({
		queryKey: ['auth', 'providers'],
		queryFn: () => api.get<AuthProviders>('/auth/providers'),
		staleTime: Infinity
	}));

	let form = $state<LoginRequest>({ username: '', password: '' });
	let submitting = $state(false);
	// svelte-ignore state_referenced_locally
	let error = $state(data.ssoError);

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		submitting = true;
		error = '';
		try {
			const me = await api.post<Me>('/auth/login', form);
			session.set(me);
			// data.next is an already-resolved, same-origin path checked by safeNext().
			// eslint-disable-next-line svelte/no-navigation-without-resolve
			await goto(data.next, { replaceState: true });
		} catch (err) {
			error = err instanceof ApiError ? err.message : 'Login failed';
			form.password = '';
		} finally {
			submitting = false;
		}
	}

	const oidcHref = $derived(`/auth/oidc/login?next=${encodeURIComponent(data.next)}`);
</script>

<svelte:head>
	<title>Sign in · degoploy</title>
</svelte:head>

<div class="flex min-h-svh items-center justify-center bg-muted/40 p-4">
	<div class="flex w-full max-w-sm flex-col gap-6">
		<div class="flex items-center justify-center gap-2">
			<Logo class="size-8" />
			<span class="text-xl font-semibold tracking-tight">degoploy</span>
		</div>

		<Card.Root>
			<Card.Header>
				<Card.Title>Sign in</Card.Title>
				<Card.Description>Webhook deployments for your projects</Card.Description>
			</Card.Header>
			<Card.Content class="flex flex-col gap-4">
				{#if error}
					<Alert.Root variant="destructive">
						<CircleAlertIcon />
						<Alert.Title>{error}</Alert.Title>
					</Alert.Root>
				{/if}

				{#if providers.isPending}
					<div class="flex justify-center py-6 text-muted-foreground">
						<LoaderIcon class="size-5 animate-spin" />
					</div>
				{:else if providers.isError}
					<Alert.Root variant="destructive">
						<CircleAlertIcon />
						<Alert.Title>{providers.error.message}</Alert.Title>
					</Alert.Root>
				{:else}
					{#if providers.data.oidc.enabled}
						<Button href={oidcHref} variant="outline" class="w-full" data-sveltekit-reload>
							<KeyRoundIcon />
							Continue with {providers.data.oidc.name}
						</Button>
					{/if}

					{#if providers.data.local && providers.data.oidc.enabled}
						<div class="flex items-center gap-3 text-xs text-muted-foreground uppercase">
							<span class="h-px flex-1 bg-border"></span>
							or
							<span class="h-px flex-1 bg-border"></span>
						</div>
					{/if}

					{#if providers.data.local}
						<form class="flex flex-col gap-4" onsubmit={submit}>
							<div class="flex flex-col gap-2">
								<Label for="username">Username</Label>
								<Input
									id="username"
									autocomplete="username"
									required
									bind:value={form.username}
									disabled={submitting}
								/>
							</div>
							<div class="flex flex-col gap-2">
								<Label for="password">Password</Label>
								<Input
									id="password"
									type="password"
									autocomplete="current-password"
									required
									bind:value={form.password}
									disabled={submitting}
								/>
							</div>
							<Button type="submit" class="w-full" disabled={submitting}>
								{#if submitting}<LoaderIcon class="animate-spin" />{/if}
								Sign in
							</Button>
						</form>
					{/if}
				{/if}
			</Card.Content>
		</Card.Root>
	</div>
</div>

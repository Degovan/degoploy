<script lang="ts" module>
	import type { Role } from '#lib/api/types.gen.js';

	export const roles: { value: Role; label: string; description: string }[] = [
		{
			value: 'admin',
			label: 'Admin',
			description: 'Everything, including users, secrets and scripts'
		},
		{
			value: 'deployer',
			label: 'Deployer',
			description: 'View projects and logs, deploy, cancel, redeploy'
		},
		{ value: 'viewer', label: 'Viewer', description: 'Read-only: projects, deployments and logs' }
	];

	export type UserFormValues = {
		username: string;
		display_name: string;
		email: string;
		role: Role;
		password: string;
		disabled: boolean;
	};
</script>

<script lang="ts">
	import { ApiError, errorMessage } from '#lib/api/client.js';
	import type { User } from '#lib/api/types.gen.js';
	import PasswordField from '#lib/components/password-field.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import { Switch } from '#lib/components/ui/switch/index.js';
	import * as Dialog from '#lib/components/ui/dialog/index.js';
	import * as Select from '#lib/components/ui/select/index.js';
	import LoaderIcon from '@lucide/svelte/icons/loader-circle';

	let {
		open = $bindable(false),
		user,
		isSelf = false,
		onsubmit
	}: {
		open?: boolean;
		/** null creates a new local user. */
		user: User | null;
		isSelf?: boolean;
		onsubmit: (values: UserFormValues) => Promise<void>;
	} = $props();

	let v = $state<UserFormValues>({
		username: '',
		display_name: '',
		email: '',
		role: 'viewer',
		password: '',
		disabled: false
	});
	let errors = $state<Record<string, string>>({});
	let formError = $state('');
	let saving = $state(false);

	$effect(() => {
		if (!open) return;
		v = user
			? {
					username: user.username,
					display_name: user.display_name,
					email: user.email,
					role: user.role,
					password: '',
					disabled: user.disabled
				}
			: {
					username: '',
					display_name: '',
					email: '',
					role: 'viewer',
					password: '',
					disabled: false
				};
		errors = {};
		formError = '';
	});

	const roleLabel = $derived(roles.find((r) => r.value === v.role)?.label ?? 'Select a role');

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		saving = true;
		errors = {};
		formError = '';
		try {
			await onsubmit({ ...v });
			open = false;
		} catch (err) {
			if (err instanceof ApiError && err.field) errors = { [err.field]: err.message };
			else formError = errorMessage(err);
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
			<Dialog.Title>{user ? `Edit ${user.username}` : 'Add user'}</Dialog.Title>
			<Dialog.Description>
				{user
					? user.auth_provider === 'oidc'
						? 'Signs in with SSO. If role sync is on, the next SSO login may overwrite the role.'
						: 'Local account.'
					: 'Creates a local account with a password.'}
			</Dialog.Description>
		</Dialog.Header>

		<form id="user-form" class="flex flex-col gap-4" onsubmit={submit}>
			{#if formError}<p class="text-sm text-destructive">{formError}</p>{/if}
			{#if !user}
				<div class="flex flex-col gap-2">
					<Label for="user-username">Username</Label>
					<Input
						id="user-username"
						required
						bind:value={v.username}
						autocomplete="off"
						aria-invalid={!!errors.username}
					/>
					{@render fieldError('username')}
				</div>
			{/if}
			<div class="flex flex-col gap-2">
				<Label for="user-display">Display name</Label>
				<Input id="user-display" bind:value={v.display_name} placeholder={v.username} />
			</div>
			<div class="flex flex-col gap-2">
				<Label for="user-email">Email</Label>
				<Input
					id="user-email"
					type="email"
					bind:value={v.email}
					autocomplete="off"
					aria-invalid={!!errors.email}
				/>
				{@render fieldError('email')}
			</div>
			<div class="flex flex-col gap-2">
				<Label for="user-role">Role</Label>
				<Select.Root type="single" bind:value={v.role as string} disabled={isSelf}>
					<Select.Trigger id="user-role" class="w-full">{roleLabel}</Select.Trigger>
					<Select.Content>
						{#each roles as r (r.value)}
							<Select.Item value={r.value} label={r.label}>
								<div class="flex flex-col">
									<span>{r.label}</span>
									<span class="text-xs text-muted-foreground">{r.description}</span>
								</div>
							</Select.Item>
						{/each}
					</Select.Content>
				</Select.Root>
				{#if isSelf}
					<p class="text-xs text-muted-foreground">You cannot change your own role.</p>
				{/if}
				{@render fieldError('role')}
			</div>
			{#if !user}
				<div class="flex flex-col gap-2">
					<Label for="user-password">Password</Label>
					<PasswordField id="user-password" bind:value={v.password} invalid={!!errors.password} />
					<p class="text-xs text-muted-foreground">At least 10 characters. Share it securely.</p>
					{@render fieldError('password')}
				</div>
			{:else if !isSelf}
				<div class="flex items-center justify-between rounded-md border p-3">
					<div class="flex flex-col gap-0.5">
						<Label for="user-disabled">Disabled</Label>
						<p class="text-xs text-muted-foreground">Signs the user out and blocks sign-in.</p>
					</div>
					<Switch id="user-disabled" bind:checked={v.disabled} />
				</div>
			{/if}
		</form>

		<Dialog.Footer>
			<Button variant="ghost" onclick={() => (open = false)} disabled={saving}>Cancel</Button>
			<Button type="submit" form="user-form" disabled={saving}>
				{#if saving}<LoaderIcon class="animate-spin" />{/if}
				{user ? 'Save' : 'Add user'}
			</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>

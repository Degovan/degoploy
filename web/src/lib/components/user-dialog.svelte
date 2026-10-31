<script lang="ts" module>
	import type { ProjectAccess, Role } from '#lib/api/types.gen.js';

	export type UserFormValues = {
		display_name: string;
		email: string;
		role: Role;
		disabled: boolean;
		selected_projects: boolean;
		projects: ProjectAccess[];
	};
</script>

<script lang="ts">
	import { ApiError, errorMessage } from '#lib/api/client.js';
	import type { User } from '#lib/api/types.gen.js';
	import ProjectAccessPicker from '#lib/components/project-access.svelte';
	import RolePicker from '#lib/components/role-picker.svelte';
	import UserAvatar from '#lib/components/user-avatar.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import { Switch } from '#lib/components/ui/switch/index.js';
	import * as Dialog from '#lib/components/ui/dialog/index.js';
	import CheckIcon from '@lucide/svelte/icons/check';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import LoaderIcon from '@lucide/svelte/icons/loader-circle';
	import Trash2Icon from '@lucide/svelte/icons/trash-2';

	let {
		open = $bindable(false),
		user,
		isSelf = false,
		ssoName = 'SSO',
		onsubmit,
		ondelete
	}: {
		open?: boolean;
		user: User | null;
		isSelf?: boolean;
		/** The identity provider's display name. */
		ssoName?: string;
		onsubmit: (values: UserFormValues) => Promise<void>;
		ondelete: (user: User) => void;
	} = $props();

	let v = $state<UserFormValues>({
		display_name: '',
		email: '',
		role: 'viewer',
		disabled: false,
		selected_projects: false,
		projects: []
	});
	let errors = $state<Record<string, string>>({});
	let formError = $state('');
	let saving = $state(false);

	// Re-seed each time the dialog opens.
	$effect(() => {
		if (!open || !user) return;
		v = {
			display_name: user.display_name,
			email: user.email,
			role: user.role,
			disabled: user.disabled,
			selected_projects: user.selected_projects,
			projects: user.projects.map((p) => ({ ...p }))
		};
		errors = {};
		formError = '';
	});

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

	const field =
		'h-10 px-3 bg-background dark:bg-background placeholder:text-muted-foreground/70 text-[13px] md:text-[13px]';
	const label = 'text-[13px] leading-[17px] text-muted-foreground';
</script>

{#snippet fieldError(key: string)}
	{#if errors[key]}<p class="text-xs/normal text-destructive">{errors[key]}</p>{/if}
{/snippet}

<Dialog.Root bind:open>
	<Dialog.Content
		showCloseButton={false}
		class="flex max-h-[calc(100svh-2rem)] flex-col gap-0 overflow-hidden rounded-xl border border-input bg-card p-0 shadow-[0_24px_64px_rgb(0_0_0/0.8)] ring-0 sm:max-w-[600px]"
	>
		{#if user}
			<form class="contents" onsubmit={submit} novalidate>
				<div
					class="flex min-h-0 flex-1 [scrollbar-width:thin] flex-col gap-5 overflow-y-auto overscroll-contain p-6"
				>
					<div class="flex items-center gap-3">
						<UserAvatar {user} class="size-10" />
						<Dialog.Header class="min-w-0 gap-1">
							<Dialog.Title class="truncate text-xl font-semibold tracking-[-0.4px]">
								Edit {user.display_name || user.username}
							</Dialog.Title>
							<Dialog.Description class="truncate text-sm text-muted-foreground">
								<span class="font-mono text-xs">@{user.username}</span> · {user.auth_provider ===
								'oidc'
									? `Signs in with ${ssoName}`
									: 'Signs in with a password'}
							</Dialog.Description>
						</Dialog.Header>
					</div>

					{#if formError}
						<p
							class="flex items-start gap-2 rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-[13px] text-destructive"
							role="alert"
						>
							<CircleAlertIcon class="mt-0.5 size-3.5 shrink-0" />
							{formError}
						</p>
					{/if}

					<div class="grid gap-4 sm:grid-cols-2">
						<div class="flex flex-col gap-2">
							<Label for="user-display" class={label}>Display name</Label>
							<Input
								id="user-display"
								bind:value={v.display_name}
								placeholder={user.username}
								class={field}
							/>
						</div>
						<div class="flex flex-col gap-2">
							<Label for="user-email" class={label}>Email</Label>
							<Input
								id="user-email"
								type="email"
								bind:value={v.email}
								autocomplete="off"
								class={field}
								aria-invalid={!!errors.email}
							/>
							{@render fieldError('email')}
						</div>
					</div>

					<div class="flex flex-col gap-2">
						<div class="flex items-center justify-between gap-3">
							<span id="user-role" class={label}>Role</span>
							{#if isSelf}
								<span class="text-xs text-muted-foreground/80">You cannot change your own role</span
								>
							{:else if user.auth_provider === 'oidc'}
								<span class="text-xs text-muted-foreground/80"
									>Role sync may overwrite it at the next sign-in</span
								>
							{/if}
						</div>
						<RolePicker bind:value={v.role} disabled={isSelf} labelledby="user-role" />
						{@render fieldError('role')}
					</div>

					<div class="flex flex-col gap-2">
						<span class={label}>Project access</span>
						{#if v.role === 'admin'}
							<p class="text-xs/normal text-muted-foreground/80">Admins see every project.</p>
						{:else}
							<ProjectAccessPicker
								bind:selected={v.selected_projects}
								bind:projects={v.projects}
								defaultRole={v.role}
								error={errors.projects || errors.selected_projects}
							/>
						{/if}
					</div>

					{#if !isSelf}
						<div class="flex flex-col overflow-hidden rounded-lg border bg-background">
							<div class="flex items-start justify-between gap-4 p-3.5">
								<div class="flex flex-col gap-1">
									<Label for="user-disabled" class="text-[13px]">Disable sign-in</Label>
									<p class="text-xs/normal text-muted-foreground/80">
										Signs them out everywhere and blocks sign-in until you turn it back on.
									</p>
								</div>
								<Switch
									id="user-disabled"
									bind:checked={v.disabled}
									class="mt-0.5 data-[size=default]:h-5 data-[size=default]:w-[34px]"
								/>
							</div>
							<div
								class="flex items-center justify-between gap-4 border-t border-red-500/25 bg-red-500/[0.06] p-3.5"
							>
								<div class="flex flex-col gap-1">
									<span class="text-[13px] font-medium">Delete user</span>
									<p class="text-xs/normal text-muted-foreground/80">
										Audit entries and deployments they triggered are kept.
									</p>
								</div>
								<Button
									type="button"
									variant="outline"
									class="h-8 shrink-0 border-red-500/40 px-3 text-[13px] text-red-500 hover:bg-red-500/10 hover:text-red-400 dark:border-red-500/40 dark:bg-transparent"
									onclick={() => {
										open = false;
										ondelete(user);
									}}
								>
									<Trash2Icon />
									Delete
								</Button>
							</div>
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
						{#if saving}<LoaderIcon class="animate-spin" />{:else}<CheckIcon />{/if}
						Save Changes
					</Button>
				</div>
			</form>
		{/if}
	</Dialog.Content>
</Dialog.Root>

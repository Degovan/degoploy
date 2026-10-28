<script lang="ts">
	import { createQuery } from '@tanstack/svelte-query';
	import { toast } from 'svelte-sonner';
	import { api, ApiError, errorMessage } from '#lib/api/client.js';
	import { inviteUrl, invitesApi } from '#lib/api/users.js';
	import type { AuthProviders, InviteLink, Role } from '#lib/api/types.gen.js';
	import { copyText } from '#lib/clipboard.js';
	import RolePicker, { roleInfo } from '#lib/components/role-picker.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import * as Dialog from '#lib/components/ui/dialog/index.js';
	import { cn } from '#lib/utils.js';
	import CheckIcon from '@lucide/svelte/icons/check';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import CopyIcon from '@lucide/svelte/icons/copy';
	import FingerprintIcon from '@lucide/svelte/icons/fingerprint-pattern';
	import LinkIcon from '@lucide/svelte/icons/link';
	import LoaderIcon from '@lucide/svelte/icons/loader-circle';
	import MailPlusIcon from '@lucide/svelte/icons/mail-plus';
	import TimerIcon from '@lucide/svelte/icons/timer';
	import XIcon from '@lucide/svelte/icons/x';

	// Two steps: the invite form, then the links it produced. Setting `links`
	// before opening shows only the second step (used after renewing an invite).
	let {
		open = $bindable(false),
		links = $bindable(null),
		oncreated
	}: {
		open?: boolean;
		links?: InviteLink[] | null;
		oncreated: () => Promise<unknown>;
	} = $props();

	const providers = createQuery(() => ({
		queryKey: ['auth', 'providers'],
		queryFn: () => api.get<AuthProviders>('/auth/providers'),
		staleTime: Infinity
	}));

	let emails = $state<string[]>([]);
	let draft = $state('');
	let role = $state<Role>('viewer');
	let errors = $state<Record<string, string>>({});
	let formError = $state('');
	let saving = $state(false);
	let copied = $state<number | 'all' | null>(null);
	let field = $state<HTMLInputElement | null>(null);

	// Mirrors the server's sanity check (internal/api/invites.go validEmail).
	const validEmail = (e: string) => /^[^\s@<>,;]+@[^\s@<>,;]+$/.test(e);
	const invalid = $derived(emails.filter((e) => !validEmail(e)));
	const people = $derived(emails.length + (draft.trim() ? 1 : 0));

	// Re-seed when the dialog opens; links survive until it closes.
	$effect(() => {
		if (open) {
			emails = [];
			draft = '';
			role = 'viewer';
			errors = {};
			formError = '';
			copied = null;
		}
	});
	// Back to the form once the close animation is over.
	$effect(() => {
		if (open || !links) return;
		const t = setTimeout(() => (links = null), 200);
		return () => clearTimeout(t);
	});

	function add(text: string) {
		const seen = emails.map((e) => e.toLowerCase());
		for (const e of text.split(/[\s,;]+/)) {
			const v = e.trim().replace(/^<|>$/g, '');
			if (v && !seen.includes(v.toLowerCase())) {
				seen.push(v.toLowerCase());
				emails.push(v);
			}
		}
		delete errors.emails;
	}

	function commitDraft() {
		if (draft.trim()) add(draft);
		draft = '';
	}

	function onkeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) return; // submits the form
		if (['Enter', ',', ';', ' ', 'Tab'].includes(e.key) && draft.trim()) {
			e.preventDefault();
			commitDraft();
		} else if (e.key === 'Enter') {
			e.preventDefault();
		} else if (e.key === 'Backspace' && draft === '' && emails.length) {
			emails.pop();
		}
	}

	// Separators that arrive without a usable keydown (mobile keyboards, IME,
	// autofill) still split the draft into chips.
	function oninput() {
		if (!/[\s,;]/.test(draft)) return;
		const parts = draft.split(/[\s,;]+/);
		const rest = parts.pop() ?? '';
		add(parts.join(' '));
		draft = rest;
	}

	function onpaste(e: ClipboardEvent) {
		const text = e.clipboardData?.getData('text') ?? '';
		if (!/[\s,;]/.test(text.trim())) return;
		e.preventDefault();
		add(draft + text);
		draft = '';
	}

	let form = $state<HTMLFormElement | null>(null);
	function onContentKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && (e.metaKey || e.ctrlKey) && form) {
			e.preventDefault();
			form.requestSubmit();
		}
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		commitDraft();
		errors = {};
		formError = '';
		if (emails.length === 0) {
			errors = { emails: 'Add at least one email address.' };
			field?.focus();
			return;
		}
		if (invalid.length) {
			errors = { emails: `${invalid[0]} is not a valid email address.` };
			return;
		}
		saving = true;
		try {
			links = await invitesApi.create({ emails, role });
			await oncreated();
		} catch (err) {
			if (err instanceof ApiError && err.field) errors = { [err.field]: err.message };
			else formError = errorMessage(err);
		} finally {
			saving = false;
		}
	}

	async function copy(text: string, which: number | 'all') {
		if (!(await copyText(text))) {
			toast.error('Could not copy to the clipboard');
			return;
		}
		copied = which;
		setTimeout(() => copied === which && (copied = null), 1500);
	}

	const allLinks = $derived(
		(links ?? []).map((l) => `${l.invite.email}: ${inviteUrl(l.token)}`).join('\n')
	);

	const modKey =
		typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.userAgent) ? '⌘' : 'Ctrl';
	const kbd = 'rounded bg-muted px-1.5 font-mono text-xs leading-4 text-muted-foreground';
</script>

<Dialog.Root bind:open>
	<Dialog.Content
		onkeydown={onContentKeydown}
		onOpenAutoFocus={(e) => {
			if (links) return;
			e.preventDefault();
			// The field mounts with the content; focus it once it is in the DOM.
			requestAnimationFrame(() => field?.focus());
		}}
		showCloseButton={false}
		class="flex max-h-[calc(100svh-2rem)] flex-col gap-0 overflow-hidden rounded-xl border border-input bg-card p-0 shadow-[0_24px_64px_rgb(0_0_0/0.8)] ring-0 sm:max-w-[600px]"
	>
		{#if links}
			<div
				class="flex min-h-0 flex-1 [scrollbar-width:thin] flex-col gap-5 overflow-y-auto overscroll-contain p-6"
			>
				<Dialog.Header class="gap-1.5">
					<Dialog.Title class="text-xl font-semibold tracking-[-0.4px]">
						{links.length === 1 ? 'Invite link ready' : `${links.length} invite links ready`}
					</Dialog.Title>
					<Dialog.Description class="text-sm text-muted-foreground">
						Send each person their link. It works once, expires in 7 days, and is not shown again.
					</Dialog.Description>
				</Dialog.Header>
				<ul class="flex flex-col overflow-hidden rounded-lg border bg-background">
					{#each links as l, i (l.invite.id)}
						{@const info = roleInfo(l.invite.role)}
						<li class="flex flex-col gap-2 border-b px-3.5 py-3 last:border-b-0">
							<div class="flex items-center justify-between gap-3">
								<span class="truncate text-[13px] font-medium">{l.invite.email}</span>
								<span
									class="inline-flex h-5 shrink-0 items-center gap-1 text-xs text-muted-foreground [&_svg]:size-3"
								>
									<info.icon />{info.label}
								</span>
							</div>
							<div class="flex items-center gap-2">
								<input
									readonly
									value={inviteUrl(l.token)}
									aria-label={`Invite link for ${l.invite.email}`}
									class="h-8 min-w-0 flex-1 rounded-md border border-input bg-card px-2.5 font-mono text-xs text-muted-foreground outline-none focus-visible:border-ring"
									onfocus={(e) => e.currentTarget.select()}
								/>
								<Button
									variant="outline"
									class="h-8 shrink-0 px-2.5 text-xs"
									onclick={() => copy(inviteUrl(l.token), i)}
								>
									{#if copied === i}<CheckIcon />Copied{:else}<CopyIcon />Copy{/if}
								</Button>
							</div>
						</li>
					{/each}
				</ul>
			</div>
			<div
				class="flex shrink-0 items-center justify-between gap-3 border-t bg-background px-6 py-4"
			>
				{#if links.length > 1}
					<Button
						variant="outline"
						class="h-9 px-3 text-[13px]"
						onclick={() => copy(allLinks, 'all')}
					>
						{#if copied === 'all'}<CheckIcon />Copied{:else}<CopyIcon />Copy all{/if}
					</Button>
				{:else}
					<span></span>
				{/if}
				<Button class="h-9 px-3.5 text-[13px]" onclick={() => (open = false)}>Done</Button>
			</div>
		{:else}
			<form bind:this={form} class="contents" onsubmit={submit} novalidate>
				<div
					class="flex min-h-0 flex-1 [scrollbar-width:thin] flex-col gap-5 overflow-y-auto overscroll-contain p-6"
				>
					<div class="flex items-start justify-between gap-4">
						<Dialog.Header class="gap-1.5">
							<Dialog.Title class="text-xl font-semibold tracking-[-0.4px]"
								>Invite people</Dialog.Title
							>
							<Dialog.Description class="text-sm text-muted-foreground">
								Each person gets a link to choose a username and password.
							</Dialog.Description>
						</Dialog.Header>
						<Dialog.Close
							class="-mt-1 -mr-1 flex size-[30px] shrink-0 items-center justify-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none [&_svg]:size-[15px]"
							aria-label="Close"
						>
							<XIcon />
						</Dialog.Close>
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

					<div class="flex flex-col gap-2">
						<div class="flex items-center justify-between">
							<label for="invite-emails" class="text-[13px] font-medium text-muted-foreground"
								>Email addresses</label
							>
							{#if people}
								<span class="text-xs text-muted-foreground/80"
									>{people} {people === 1 ? 'person' : 'people'}</span
								>
							{/if}
						</div>
						<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
						<div
							class={cn(
								'flex min-h-11 cursor-text flex-wrap items-center gap-1.5 rounded-md border border-input bg-background px-2 py-[7px] transition-[color,box-shadow] focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/40',
								errors.emails && 'border-destructive focus-within:border-destructive'
							)}
							onclick={() => field?.focus()}
						>
							{#each emails as e, i (e)}
								{@const bad = !validEmail(e)}
								<span
									class={cn(
										'inline-flex h-7 max-w-full items-center gap-1.5 rounded-[5px] border pr-1.5 pl-2.5 text-[13px]',
										bad
											? 'border-destructive/50 bg-destructive/10 text-destructive'
											: 'border-input bg-muted'
									)}
								>
									<span class="truncate">{e}</span>
									<button
										type="button"
										class="rounded-sm text-muted-foreground hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:outline-none [&_svg]:size-3"
										aria-label={`Remove ${e}`}
										onclick={(ev) => {
											ev.stopPropagation();
											emails.splice(i, 1);
											delete errors.emails;
											field?.focus();
										}}
									>
										<XIcon />
									</button>
								</span>
							{/each}
							<input
								bind:this={field}
								bind:value={draft}
								id="invite-emails"
								type="email"
								multiple
								autocomplete="off"
								spellcheck={false}
								placeholder={emails.length ? 'Add another email…' : 'name@example.com'}
								class="h-7 min-w-40 flex-1 bg-transparent px-1 text-sm outline-none placeholder:text-muted-foreground/70"
								aria-invalid={!!errors.emails}
								aria-describedby="invite-emails-hint"
								{onkeydown}
								{oninput}
								{onpaste}
								onblur={commitDraft}
							/>
						</div>
						{#if errors.emails}
							<p class="text-xs/normal text-destructive" role="alert">{errors.emails}</p>
						{:else}
							<p id="invite-emails-hint" class="text-xs/normal text-muted-foreground/80">
								Paste a list or press Enter after each address. Everyone gets the same role.
							</p>
						{/if}
					</div>

					<div class="flex flex-col gap-2">
						<div class="flex items-center justify-between gap-3">
							<span id="invite-role" class="text-[13px] font-medium text-muted-foreground"
								>Role</span
							>
							<span class="text-xs text-muted-foreground/80"
								>Applies to everyone in this invite</span
							>
						</div>
						<RolePicker bind:value={role} labelledby="invite-role" />
						{#if errors.role}<p class="text-xs/normal text-destructive">{errors.role}</p>{/if}
					</div>

					<dl class="flex flex-col overflow-hidden rounded-lg border bg-background text-xs">
						{#snippet row(Icon: typeof LinkIcon, key: string, value: string)}
							<div class="flex items-center gap-2.5 border-b px-3.5 py-2.5 last:border-b-0">
								<Icon class="size-3.5 shrink-0 text-muted-foreground/80" />
								<dt class="shrink-0 font-medium text-muted-foreground">{key}</dt>
								<dd class="min-w-0 truncate text-muted-foreground/80">{value}</dd>
							</div>
						{/snippet}
						{@render row(LinkIcon, 'Delivery', 'you copy each link and send it yourself')}
						{@render row(TimerIcon, 'Link expires', 'in 7 days')}
						{#if providers.data?.oidc.enabled}
							{@render row(
								FingerprintIcon,
								`${providers.data.oidc.name} members`,
								'can sign in directly — no invite needed'
							)}
						{/if}
					</dl>
				</div>

				<div
					class="flex shrink-0 items-center justify-between gap-3 border-t bg-background px-6 py-4"
				>
					<Dialog.Close>
						{#snippet child({ props })}
							<Button {...props} variant="outline" class="h-9 px-3 text-[13px]" disabled={saving}>
								Cancel
							</Button>
						{/snippet}
					</Dialog.Close>
					<div class="flex items-center gap-3">
						<span class="hidden items-center gap-1 sm:flex" aria-hidden="true">
							<kbd class={kbd}>{modKey}</kbd><kbd class={kbd}>↵</kbd>
						</span>
						<Button type="submit" class="h-9 px-3.5 text-[13px]" disabled={saving}>
							{#if saving}<LoaderIcon class="animate-spin" />{:else}<MailPlusIcon />{/if}
							{people > 1 ? `Invite ${people} people` : 'Create invite'}
						</Button>
					</div>
				</div>
			</form>
		{/if}
	</Dialog.Content>
</Dialog.Root>

<script lang="ts">
	import { ApiError, errorMessage } from '#lib/api/client.js';
	import type { Variable, VariableInput } from '#lib/api/types.gen.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import { Switch } from '#lib/components/ui/switch/index.js';
	import { Textarea } from '#lib/components/ui/textarea/index.js';
	import * as Dialog from '#lib/components/ui/dialog/index.js';
	import { cn } from '#lib/utils.js';
	import CheckIcon from '@lucide/svelte/icons/check';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import EyeIcon from '@lucide/svelte/icons/eye';
	import EyeOffIcon from '@lucide/svelte/icons/eye-off';
	import LoaderIcon from '@lucide/svelte/icons/loader-circle';
	import LockIcon from '@lucide/svelte/icons/lock';
	import PlusIcon from '@lucide/svelte/icons/plus';

	// `editing` null creates a variable; otherwise it edits that one.
	let {
		open = $bindable(false),
		editing,
		onsubmit
	}: {
		open?: boolean;
		editing: Variable | null;
		onsubmit: (input: VariableInput) => Promise<void>;
	} = $props();

	let name = $state('');
	let value = $state('');
	let secret = $state(false);
	let visible = $state(true);
	let errors = $state<Record<string, string>>({});
	let formError = $state('');
	let saving = $state(false);

	// A saved secret cannot be read back: its field starts empty and empty keeps it.
	const keepsSecret = $derived(!!editing?.secret && secret);
	const mustRetype = $derived(!!editing?.secret && !secret);

	// Re-seed each time the dialog opens.
	$effect(() => {
		if (!open) return;
		name = editing?.name ?? '';
		value = editing && !editing.secret ? editing.value : '';
		secret = editing?.secret ?? false;
		visible = !editing?.secret;
		errors = {};
		formError = '';
	});

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		errors = {};
		formError = '';
		const input: VariableInput = { name: name.trim(), secret };
		if (!(keepsSecret && value === '')) input.value = value;
		saving = true;
		try {
			await onsubmit(input);
			open = false;
		} catch (err) {
			if (err instanceof ApiError && err.field) errors = { [err.field]: err.message };
			else formError = errorMessage(err);
		} finally {
			saving = false;
		}
	}

	const field =
		'bg-background dark:bg-background placeholder:text-muted-foreground/70 text-[13px] md:text-[13px] font-mono';
</script>

{#snippet fieldError(key: string)}
	{#if errors[key]}<p class="text-xs/normal text-destructive">{errors[key]}</p>{/if}
{/snippet}

<Dialog.Root bind:open>
	<Dialog.Content
		showCloseButton={false}
		class="flex max-h-[calc(100svh-2rem)] flex-col gap-0 overflow-hidden rounded-xl border border-input bg-card p-0 shadow-[0_24px_64px_rgb(0_0_0/0.8)] ring-0 sm:max-w-[520px]"
	>
		<form class="contents" onsubmit={submit}>
			<div
				class="flex min-h-0 flex-1 [scrollbar-width:thin] flex-col gap-5 overflow-y-auto overscroll-contain p-6"
			>
				<Dialog.Header class="gap-1.5">
					<Dialog.Title class="text-xl font-semibold tracking-[-0.4px]">
						{editing ? 'Edit Variable' : 'New Variable'}
					</Dialog.Title>
					<Dialog.Description class="text-sm text-muted-foreground">
						Passed to the deploy script on every run of this project.
					</Dialog.Description>
				</Dialog.Header>

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
					<Label for="var-name" class="text-[13px] leading-[17px] text-muted-foreground">Name</Label
					>
					<Input
						id="var-name"
						required
						bind:value={name}
						placeholder="DB_PASSWORD"
						autocomplete="off"
						spellcheck={false}
						class={cn(field, 'h-10 px-3')}
						aria-invalid={!!errors.name}
					/>
					<p class="text-xs/normal text-muted-foreground/80">
						Letters, digits and underscores, not starting with a digit.
					</p>
					{@render fieldError('name')}
				</div>

				<div class="flex flex-col gap-2">
					<div class="flex items-center justify-between">
						<Label for="var-value" class="text-[13px] leading-[17px] text-muted-foreground"
							>Value</Label
						>
						{#if secret}
							<button
								type="button"
								class="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground [&_svg]:size-3.5"
								onclick={() => (visible = !visible)}
							>
								{#if visible}<EyeOffIcon />Hide{:else}<EyeIcon />Show{/if}
							</button>
						{/if}
					</div>
					<Textarea
						id="var-value"
						bind:value
						rows={3}
						required={!keepsSecret}
						autocomplete="off"
						spellcheck={false}
						placeholder={keepsSecret ? 'Leave empty to keep the current value' : ''}
						class={cn(
							field,
							'max-h-60 min-h-[76px] [scrollbar-width:thin] px-3 py-2.5 leading-[1.6] break-all',
							secret && !visible && '[-webkit-text-security:disc]'
						)}
						aria-invalid={!!errors.value}
					/>
					{#if mustRetype}
						<p class="text-xs/normal text-muted-foreground/80">
							The current secret cannot be shown. Enter the value to store it as plain text.
						</p>
					{/if}
					{@render fieldError('value')}
				</div>

				<div class="flex items-start justify-between gap-4 rounded-lg border bg-background p-3.5">
					<div class="flex flex-col gap-1">
						<Label for="var-secret" class="flex items-center gap-1.5 text-[13px]">
							<LockIcon class="size-3.5 text-muted-foreground" />
							Secret
						</Label>
						<p class="text-xs/normal text-muted-foreground/80">
							Encrypted at rest and never shown again after saving. Its value is masked as *** in
							deploy logs.
						</p>
					</div>
					<Switch
						id="var-secret"
						bind:checked={secret}
						onCheckedChange={(on) => (visible = !on)}
						class="mt-0.5 data-[size=default]:h-5 data-[size=default]:w-[34px]"
					/>
				</div>
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
					{#if saving}
						<LoaderIcon class="animate-spin" />
					{:else if editing}
						<CheckIcon />
					{:else}
						<PlusIcon />
					{/if}
					{editing ? 'Save Changes' : 'Create Variable'}
				</Button>
			</div>
		</form>
	</Dialog.Content>
</Dialog.Root>

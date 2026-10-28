<script lang="ts" module>
	import type { RuleInput, TriggerRule } from '#lib/api/types.gen.js';

	export const emptyRule = (): RuleInput => ({
		name: '',
		enabled: true,
		event: 'push',
		ref_pattern: '',
		paths_include: [],
		paths_exclude: [],
		skip_keyword: '[skip deploy]',
		extra_env: {}
	});

	export const ruleToInput = (r: TriggerRule): RuleInput => ({
		name: r.name,
		enabled: r.enabled,
		event: r.event,
		ref_pattern: r.ref_pattern,
		paths_include: [...r.paths_include],
		paths_exclude: [...r.paths_exclude],
		skip_keyword: r.skip_keyword,
		extra_env: { ...r.extra_env }
	});
</script>

<script lang="ts">
	import { ApiError, errorMessage } from '#lib/api/client.js';
	import EventToggle from '#lib/components/event-toggle.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import { Switch } from '#lib/components/ui/switch/index.js';
	import { Textarea } from '#lib/components/ui/textarea/index.js';
	import * as Dialog from '#lib/components/ui/dialog/index.js';
	import { cn } from '#lib/utils.js';
	import CheckIcon from '@lucide/svelte/icons/check';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import LoaderIcon from '@lucide/svelte/icons/loader-circle';
	import PlusIcon from '@lucide/svelte/icons/plus';

	let {
		open = $bindable(false),
		initial,
		mode,
		onsubmit
	}: {
		open?: boolean;
		initial: RuleInput;
		mode: 'create' | 'edit';
		onsubmit: (rule: RuleInput) => Promise<void>;
	} = $props();

	// Shared look of the dialog's fields: black wells on the card, design-sized type.
	const field =
		'bg-background dark:bg-background placeholder:text-muted-foreground/70 text-[13px] md:text-[13px]';
	const input = cn(field, 'h-10 px-3');
	const area = cn(
		field,
		'min-h-[76px] max-h-48 px-3 py-2.5 font-mono leading-[1.6] [scrollbar-width:thin]'
	);

	// Text-area friendly copies of the list/map fields.
	let v = $state(emptyRule());
	let include = $state('');
	let exclude = $state('');
	let env = $state('');
	let errors = $state<Record<string, string>>({});
	let formError = $state('');
	let saving = $state(false);

	// Re-seed each time the dialog opens.
	$effect(() => {
		if (!open) return;
		v = { ...initial };
		include = initial.paths_include.join('\n');
		exclude = initial.paths_exclude.join('\n');
		env = Object.entries(initial.extra_env)
			.map(([k, val]) => `${k}=${val}`)
			.join('\n');
		errors = {};
		formError = '';
	});

	const lines = (s: string) =>
		s
			.split('\n')
			.map((l) => l.trim())
			.filter(Boolean);

	function parseEnv(): Record<string, string> | null {
		const out: Record<string, string> = {};
		for (const line of lines(env)) {
			const i = line.indexOf('=');
			if (i <= 0) {
				errors = { extra_env: `"${line}" is not KEY=value` };
				return null;
			}
			out[line.slice(0, i).trim()] = line.slice(i + 1);
		}
		return out;
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		errors = {};
		formError = '';
		const extra_env = parseEnv();
		if (!extra_env) return;
		saving = true;
		try {
			await onsubmit({
				...v,
				paths_include: lines(include),
				paths_exclude: lines(exclude),
				extra_env
			});
			open = false;
		} catch (err) {
			if (err instanceof ApiError && err.field) errors = { [err.field]: err.message };
			else formError = errorMessage(err);
		} finally {
			saving = false;
		}
	}
</script>

{#snippet label(id: string, text: string)}
	<Label for={id} class="text-[13px] leading-[17px] text-muted-foreground">{text}</Label>
{/snippet}

{#snippet hint(text: string)}
	<p class="text-xs/normal text-muted-foreground/80">{text}</p>
{/snippet}

{#snippet fieldError(name: string)}
	{#if errors[name]}
		<p class="text-xs/normal text-destructive" id={`rule-${name}-error`}>{errors[name]}</p>
	{/if}
{/snippet}

<Dialog.Root bind:open>
	<!-- Only the body scrolls, so the rounded frame and the footer stay put. -->
	<Dialog.Content
		showCloseButton={false}
		class="flex max-h-[calc(100svh-2rem)] flex-col gap-0 overflow-hidden rounded-xl border border-input bg-card p-0 shadow-[0_24px_64px_rgb(0_0_0/0.8)] ring-0 sm:max-w-[600px]"
	>
		<form class="contents" onsubmit={submit}>
			<div
				class="flex min-h-0 flex-1 [scrollbar-width:thin] flex-col gap-5 overflow-y-auto overscroll-contain p-6"
			>
				<Dialog.Header class="gap-1.5">
					<Dialog.Title class="text-xl font-semibold tracking-[-0.4px]">
						{mode === 'edit' ? 'Edit Rule' : 'New Rule'}
					</Dialog.Title>
					<Dialog.Description class="text-sm text-muted-foreground">
						A push deploys when every condition of at least one enabled rule is met.
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

				<div class="grid gap-x-4 gap-y-5 sm:grid-cols-[minmax(0,1fr)_220px]">
					<div class="flex flex-col gap-2">
						{@render label('rule-name', 'Name')}
						<Input
							id="rule-name"
							required
							bind:value={v.name}
							placeholder="Production"
							class={cn(input, 'text-sm md:text-sm')}
							aria-invalid={!!errors.name}
						/>
						{@render fieldError('name')}
					</div>
					<div class="flex flex-col gap-2">
						<span
							id="rule-event"
							class="text-[13px] leading-[17px] font-medium text-muted-foreground">Trigger on</span
						>
						<EventToggle bind:value={v.event} labelledby="rule-event" class="h-10" />
						{@render fieldError('event')}
					</div>
				</div>

				<div class="flex flex-col gap-2">
					{@render label('rule-ref', v.event === 'tag' ? 'Tag pattern' : 'Branch pattern')}
					<Input
						id="rule-ref"
						required
						bind:value={v.ref_pattern}
						class={cn(input, 'font-mono')}
						placeholder={v.event === 'tag' ? 'v*' : 'main, release/*'}
						aria-invalid={!!errors.ref_pattern}
					/>
					{@render hint('Comma-separated. * matches within a segment, ** across /.')}
					{@render fieldError('ref_pattern')}
				</div>

				<div class="flex flex-col gap-2">
					<div class="grid gap-x-4 gap-y-5 sm:grid-cols-2">
						<div class="flex flex-col gap-2">
							{@render label('rule-include', 'Only when these paths change')}
							<Textarea
								id="rule-include"
								bind:value={include}
								rows={2}
								class={area}
								placeholder="src/**&#10;composer.lock"
								aria-invalid={!!errors.paths_include}
							/>
							{@render fieldError('paths_include')}
						</div>
						<div class="flex flex-col gap-2">
							{@render label('rule-exclude', 'Ignore changes to')}
							<Textarea
								id="rule-exclude"
								bind:value={exclude}
								rows={2}
								class={area}
								placeholder="*.md&#10;docs/**"
								aria-invalid={!!errors.paths_exclude}
							/>
							{@render fieldError('paths_exclude')}
						</div>
					</div>
					{@render hint(
						'One pattern per line, both optional. Skipped when a push does not list its changed files.'
					)}
				</div>

				<div class="flex flex-col gap-2">
					{@render label('rule-skip', 'Skip when commit message contains')}
					<Input
						id="rule-skip"
						bind:value={v.skip_keyword}
						class={cn(input, 'font-mono')}
						placeholder="[skip deploy]"
						aria-invalid={!!errors.skip_keyword}
					/>
					{@render fieldError('skip_keyword')}
				</div>

				<div class="flex flex-col gap-2">
					{@render label('rule-env', 'Extra environment')}
					<Textarea
						id="rule-env"
						bind:value={env}
						rows={2}
						class={area}
						placeholder="APP_ENV=production"
						aria-invalid={!!errors.extra_env}
					/>
					{@render hint('KEY=value per line. The rule name is available as DEPLOY_RULE.')}
					{@render fieldError('extra_env')}
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
				<div class="flex items-center gap-4">
					<div class="flex items-center gap-2">
						<Switch
							id="rule-enabled"
							bind:checked={v.enabled}
							class="data-[size=default]:h-5 data-[size=default]:w-[34px]"
						/>
						<Label
							for="rule-enabled"
							class="cursor-pointer text-[13px] font-normal text-muted-foreground">Enabled</Label
						>
					</div>
					<Button type="submit" class="h-9 px-3.5 text-[13px]" disabled={saving}>
						{#if saving}
							<LoaderIcon class="animate-spin" />
						{:else if mode === 'edit'}
							<CheckIcon />
						{:else}
							<PlusIcon />
						{/if}
						{mode === 'edit' ? 'Save Changes' : 'Create Rule'}
					</Button>
				</div>
			</div>
		</form>
	</Dialog.Content>
</Dialog.Root>

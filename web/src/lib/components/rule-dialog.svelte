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
	import { Button } from '#lib/components/ui/button/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import { Switch } from '#lib/components/ui/switch/index.js';
	import { Textarea } from '#lib/components/ui/textarea/index.js';
	import * as Dialog from '#lib/components/ui/dialog/index.js';
	import * as RadioGroup from '#lib/components/ui/radio-group/index.js';
	import LoaderIcon from '@lucide/svelte/icons/loader-circle';

	let {
		open = $bindable(false),
		initial,
		title,
		onsubmit
	}: {
		open?: boolean;
		initial: RuleInput;
		title: string;
		onsubmit: (rule: RuleInput) => Promise<void>;
	} = $props();

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

{#snippet fieldError(name: string)}
	{#if errors[name]}<p class="text-sm text-destructive">{errors[name]}</p>{/if}
{/snippet}

<Dialog.Root bind:open>
	<Dialog.Content class="max-h-[90svh] overflow-y-auto sm:max-w-xl">
		<Dialog.Header>
			<Dialog.Title>{title}</Dialog.Title>
			<Dialog.Description>
				A push deploys when every condition of at least one enabled rule is met.
			</Dialog.Description>
		</Dialog.Header>

		<form id="rule-form" class="flex flex-col gap-5" onsubmit={submit}>
			{#if formError}<p class="text-sm text-destructive">{formError}</p>{/if}

			<div class="flex flex-col gap-2">
				<Label for="rule-name">Name</Label>
				<Input id="rule-name" required bind:value={v.name} placeholder="Production" />
				{@render fieldError('name')}
			</div>

			<div class="flex flex-col gap-2">
				<Label>Trigger on</Label>
				<RadioGroup.Root bind:value={v.event} class="flex gap-6">
					<div class="flex items-center gap-2">
						<RadioGroup.Item value="push" id="event-push" />
						<Label for="event-push" class="font-normal">Branch push</Label>
					</div>
					<div class="flex items-center gap-2">
						<RadioGroup.Item value="tag" id="event-tag" />
						<Label for="event-tag" class="font-normal">Tag push</Label>
					</div>
				</RadioGroup.Root>
			</div>

			<div class="flex flex-col gap-2">
				<Label for="rule-ref">{v.event === 'tag' ? 'Tag pattern' : 'Branch pattern'}</Label>
				<Input
					id="rule-ref"
					required
					bind:value={v.ref_pattern}
					class="font-mono"
					placeholder={v.event === 'tag' ? 'v*' : 'main, release/*'}
				/>
				<p class="text-xs text-muted-foreground">
					Comma-separated. <code>*</code> matches within a segment, <code>**</code> across
					<code>/</code>.
				</p>
				{@render fieldError('ref_pattern')}
			</div>

			<div class="grid gap-4 sm:grid-cols-2">
				<div class="flex flex-col gap-2">
					<Label for="rule-include">Only when these paths change</Label>
					<Textarea
						id="rule-include"
						bind:value={include}
						rows={3}
						class="font-mono text-xs"
						placeholder="src/**&#10;composer.lock"
					/>
					{@render fieldError('paths_include')}
				</div>
				<div class="flex flex-col gap-2">
					<Label for="rule-exclude">Ignore changes to</Label>
					<Textarea
						id="rule-exclude"
						bind:value={exclude}
						rows={3}
						class="font-mono text-xs"
						placeholder="*.md&#10;docs/**"
					/>
					{@render fieldError('paths_exclude')}
				</div>
			</div>
			<p class="-mt-3 text-xs text-muted-foreground">
				One pattern per line, both optional. If the push does not list changed files (new branch,
				force push) path filters are skipped.
			</p>

			<div class="flex flex-col gap-2">
				<Label for="rule-skip">Skip when commit message contains</Label>
				<Input
					id="rule-skip"
					bind:value={v.skip_keyword}
					class="font-mono"
					placeholder="[skip deploy]"
				/>
				{@render fieldError('skip_keyword')}
			</div>

			<div class="flex flex-col gap-2">
				<Label for="rule-env">Extra environment</Label>
				<Textarea
					id="rule-env"
					bind:value={env}
					rows={3}
					class="font-mono text-xs"
					placeholder="APP_ENV=production"
				/>
				<p class="text-xs text-muted-foreground">
					<code>KEY=value</code> per line, passed to the deploy script. The matched rule's name is
					in
					<code>DEPLOY_RULE</code>.
				</p>
				{@render fieldError('extra_env')}
			</div>

			<div class="flex items-center justify-between rounded-md border p-3">
				<Label for="rule-enabled">Enabled</Label>
				<Switch id="rule-enabled" bind:checked={v.enabled} />
			</div>
		</form>

		<Dialog.Footer>
			<Button variant="ghost" onclick={() => (open = false)} disabled={saving}>Cancel</Button>
			<Button type="submit" form="rule-form" disabled={saving}>
				{#if saving}<LoaderIcon class="animate-spin" />{/if}
				Save rule
			</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>

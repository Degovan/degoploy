<script lang="ts">
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { toast } from 'svelte-sonner';
	import { ApiError, errorMessage } from '#lib/api/client.js';
	import { ruleKeys, rulesApi } from '#lib/api/rules.js';
	import type {
		RuleInput,
		RuleTestResponse,
		TriggerEvent,
		TriggerRule
	} from '#lib/api/types.gen.js';
	import ConfirmButton from '#lib/components/confirm-button.svelte';
	import RuleDialog, { emptyRule, ruleToInput } from '#lib/components/rule-dialog.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Badge } from '#lib/components/ui/badge/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import { Switch } from '#lib/components/ui/switch/index.js';
	import { Textarea } from '#lib/components/ui/textarea/index.js';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import * as Card from '#lib/components/ui/card/index.js';
	import * as Alert from '#lib/components/ui/alert/index.js';
	import * as RadioGroup from '#lib/components/ui/radio-group/index.js';
	import { session } from '#lib/stores/session.svelte.js';
	import { cn } from '#lib/utils.js';
	import ArrowDownIcon from '@lucide/svelte/icons/arrow-down';
	import ArrowUpIcon from '@lucide/svelte/icons/arrow-up';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import CircleCheckIcon from '@lucide/svelte/icons/circle-check';
	import CircleMinusIcon from '@lucide/svelte/icons/circle-minus';
	import FlaskConicalIcon from '@lucide/svelte/icons/flask-conical';
	import LoaderIcon from '@lucide/svelte/icons/loader-circle';
	import PencilIcon from '@lucide/svelte/icons/pencil';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import Trash2Icon from '@lucide/svelte/icons/trash-2';
	import ZapIcon from '@lucide/svelte/icons/zap';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	const queryClient = useQueryClient();
	const canEdit = $derived(session.can('project.edit'));

	const rules = createQuery(() => ({
		queryKey: ruleKeys.list(data.id),
		queryFn: () => rulesApi.list(data.id)
	}));

	const setRules = (list: TriggerRule[]) => queryClient.setQueryData(ruleKeys.list(data.id), list);
	const refresh = () => queryClient.invalidateQueries({ queryKey: ruleKeys.list(data.id) });

	// Dialog state: `editing` null means "new rule".
	let dialogOpen = $state(false);
	let editing = $state<TriggerRule | null>(null);
	const dialogInitial = $derived(editing ? ruleToInput(editing) : emptyRule());

	function openNew() {
		editing = null;
		dialogOpen = true;
	}
	function openEdit(r: TriggerRule) {
		editing = r;
		dialogOpen = true;
	}

	async function saveRule(input: RuleInput) {
		if (editing) await rulesApi.update(data.id, editing.id, input);
		else await rulesApi.create(data.id, input);
		await refresh();
		toast.success(editing ? 'Rule saved' : 'Rule added');
	}

	async function toggle(r: TriggerRule, enabled: boolean) {
		try {
			await rulesApi.update(data.id, r.id, { ...ruleToInput(r), enabled });
			await refresh();
		} catch (err) {
			toast.error(errorMessage(err));
			await refresh();
		}
	}

	async function move(index: number, delta: -1 | 1) {
		const list = [...(rules.data ?? [])];
		const target = index + delta;
		if (target < 0 || target >= list.length) return;
		[list[index], list[target]] = [list[target]!, list[index]!];
		try {
			setRules(
				await rulesApi.reorder(
					data.id,
					list.map((r) => r.id)
				)
			);
		} catch (err) {
			toast.error(errorMessage(err));
		}
	}

	async function remove(r: TriggerRule) {
		try {
			await rulesApi.remove(data.id, r.id);
			await refresh();
			toast.success(`Rule "${r.name}" deleted`);
		} catch (err) {
			toast.error(errorMessage(err));
		}
	}

	// Simulator
	let sim = $state({ event: 'push' as TriggerEvent, ref: 'main', message: '', files: '' });
	let simResult = $state<RuleTestResponse | null>(null);
	let simError = $state('');
	let simRunning = $state(false);

	async function simulate(e: SubmitEvent) {
		e.preventDefault();
		simRunning = true;
		simError = '';
		try {
			simResult = await rulesApi.test(data.id, {
				event: sim.event,
				ref: sim.ref,
				message: sim.message,
				files: sim.files.split('\n')
			});
		} catch (err) {
			simResult = null;
			simError = err instanceof ApiError ? err.message : errorMessage(err);
		} finally {
			simRunning = false;
		}
	}
</script>

<div class="grid gap-6 xl:grid-cols-[1fr_22rem]">
	<Card.Root class="min-w-0">
		<Card.Header>
			<Card.Title>Trigger rules</Card.Title>
			<Card.Description>
				Evaluated top to bottom; the first rule that matches a push starts a deployment.
			</Card.Description>
			{#if canEdit}
				<Card.Action>
					<Button size="sm" onclick={openNew}>
						<PlusIcon />
						Add rule
					</Button>
				</Card.Action>
			{/if}
		</Card.Header>
		<Card.Content class="flex flex-col gap-3">
			{#if rules.isPending}
				<Skeleton class="h-24 w-full" />
			{:else if rules.isError}
				<Alert.Root variant="destructive">
					<CircleAlertIcon />
					<Alert.Title>{rules.error.message}</Alert.Title>
				</Alert.Root>
			{:else if rules.data.length === 0}
				<div class="rounded-md border border-dashed p-8 text-center text-sm text-muted-foreground">
					<ZapIcon class="mx-auto mb-2 size-5" />
					No rules yet, so webhooks never deploy. Add a rule such as “push to main”.
				</div>
			{:else}
				{#each rules.data as r, i (r.id)}
					<div class={cn('flex flex-col gap-3 rounded-lg border p-4', !r.enabled && 'opacity-60')}>
						<div class="flex flex-wrap items-center gap-2">
							<span class="text-xs font-medium text-muted-foreground tabular-nums">#{i + 1}</span>
							<span class="font-medium">{r.name}</span>
							<Badge variant="outline">{r.event === 'tag' ? 'Tag push' : 'Branch push'}</Badge>
							{#if !r.enabled}<Badge variant="secondary">Disabled</Badge>{/if}
							{#if canEdit}
								<div class="ml-auto flex items-center gap-1">
									<Switch
										checked={r.enabled}
										onCheckedChange={(v) => toggle(r, v)}
										aria-label={`Enable ${r.name}`}
									/>
									<Button
										variant="ghost"
										size="icon-sm"
										disabled={i === 0}
										onclick={() => move(i, -1)}
										aria-label="Move up"><ArrowUpIcon /></Button
									>
									<Button
										variant="ghost"
										size="icon-sm"
										disabled={i === rules.data.length - 1}
										onclick={() => move(i, 1)}
										aria-label="Move down"><ArrowDownIcon /></Button
									>
									<Button
										variant="ghost"
										size="icon-sm"
										onclick={() => openEdit(r)}
										aria-label="Edit"
									>
										<PencilIcon />
									</Button>
									<ConfirmButton
										variant="ghost"
										size="icon-sm"
										label={`Delete rule ${r.name}`}
										title={`Delete rule "${r.name}"?`}
										description="Pushes that only this rule matched will no longer deploy."
										confirmLabel="Delete rule"
										destructive
										onconfirm={() => remove(r)}
									>
										<Trash2Icon />
									</ConfirmButton>
								</div>
							{/if}
						</div>
						<dl class="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-2">
							<div>
								<dt class="text-xs text-muted-foreground">
									{r.event === 'tag' ? 'Tags' : 'Branches'}
								</dt>
								<dd class="font-mono text-xs">{r.ref_pattern}</dd>
							</div>
							{#if r.skip_keyword}
								<div>
									<dt class="text-xs text-muted-foreground">Skip keyword</dt>
									<dd class="font-mono text-xs">{r.skip_keyword}</dd>
								</div>
							{/if}
							{#if r.paths_include.length}
								<div>
									<dt class="text-xs text-muted-foreground">Only paths</dt>
									<dd class="font-mono text-xs">{r.paths_include.join(', ')}</dd>
								</div>
							{/if}
							{#if r.paths_exclude.length}
								<div>
									<dt class="text-xs text-muted-foreground">Ignored paths</dt>
									<dd class="font-mono text-xs">{r.paths_exclude.join(', ')}</dd>
								</div>
							{/if}
							{#if Object.keys(r.extra_env).length}
								<div class="sm:col-span-2">
									<dt class="text-xs text-muted-foreground">Environment</dt>
									<dd class="font-mono text-xs">
										{Object.entries(r.extra_env)
											.map(([k, v]) => `${k}=${v}`)
											.join('  ')}
									</dd>
								</div>
							{/if}
						</dl>
					</div>
				{/each}
			{/if}
		</Card.Content>
	</Card.Root>

	<Card.Root class="h-fit">
		<Card.Header>
			<Card.Title class="flex items-center gap-2 text-base">
				<FlaskConicalIcon class="size-4" />
				Test a push
			</Card.Title>
			<Card.Description>See which rule would match, without deploying.</Card.Description>
		</Card.Header>
		<Card.Content>
			<form class="flex flex-col gap-4" onsubmit={simulate}>
				<RadioGroup.Root bind:value={sim.event} class="flex gap-6">
					<div class="flex items-center gap-2">
						<RadioGroup.Item value="push" id="sim-push" />
						<Label for="sim-push" class="font-normal">Branch</Label>
					</div>
					<div class="flex items-center gap-2">
						<RadioGroup.Item value="tag" id="sim-tag" />
						<Label for="sim-tag" class="font-normal">Tag</Label>
					</div>
				</RadioGroup.Root>
				<div class="flex flex-col gap-2">
					<Label for="sim-ref">{sim.event === 'tag' ? 'Tag name' : 'Branch name'}</Label>
					<Input id="sim-ref" required bind:value={sim.ref} class="font-mono" />
				</div>
				<div class="flex flex-col gap-2">
					<Label for="sim-msg">Commit message</Label>
					<Input id="sim-msg" bind:value={sim.message} placeholder="fix: checkout bug" />
				</div>
				<div class="flex flex-col gap-2">
					<Label for="sim-files">Changed files</Label>
					<Textarea
						id="sim-files"
						bind:value={sim.files}
						rows={3}
						class="font-mono text-xs"
						placeholder="src/app.go&#10;README.md"
					/>
					<p class="text-xs text-muted-foreground">One per line; leave empty for “unknown”.</p>
				</div>
				<Button type="submit" variant="outline" disabled={simRunning}>
					{#if simRunning}<LoaderIcon class="animate-spin" />{/if}
					Test
				</Button>
			</form>

			{#if simError}
				<p class="mt-4 text-sm text-destructive">{simError}</p>
			{:else if simResult}
				<div class="mt-4 flex flex-col gap-2" data-testid="sim-results">
					<p class="text-sm font-medium">
						{simResult.matched_rule_id ? 'Would deploy' : 'Would not deploy'}
					</p>
					{#each simResult.results as res (res.rule_id)}
						<div class="flex gap-2 text-sm">
							{#if res.matched}
								<CircleCheckIcon class="mt-0.5 size-4 shrink-0 text-emerald-600" />
							{:else}
								<CircleMinusIcon class="mt-0.5 size-4 shrink-0 text-muted-foreground" />
							{/if}
							<div>
								<span class="font-medium">{res.name}</span>
								<p class="text-xs text-muted-foreground">{res.reason}</p>
							</div>
						</div>
					{:else}
						<p class="text-sm text-muted-foreground">No rules configured.</p>
					{/each}
				</div>
			{/if}
		</Card.Content>
	</Card.Root>
</div>

<RuleDialog
	bind:open={dialogOpen}
	initial={dialogInitial}
	title={editing ? `Edit rule "${editing.name}"` : 'New rule'}
	onsubmit={saveRule}
/>

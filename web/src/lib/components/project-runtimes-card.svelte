<script lang="ts">
	import { untrack } from 'svelte';
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { toast } from 'svelte-sonner';
	import { ApiError, errorMessage } from '#lib/api/client.js';
	import { projectKeys, projectsApi } from '#lib/api/projects.js';
	import { runtimeKeys, runtimesApi, runtimeTitles } from '#lib/api/runtimes.js';
	import type { Project } from '#lib/api/types.gen.js';
	import RuntimeSettingRow from '#lib/components/runtime-setting-row.svelte';
	import SettingsCard from '#lib/components/settings-card.svelte';
	import { session } from '#lib/stores/session.svelte.js';

	// The Runtimes card of project settings: Off, Auto or a version for each runtime.
	let { project, dirty = $bindable(false) }: { project: Project; dirty?: boolean } = $props();

	const queryClient = useQueryClient();
	const canManage = $derived(session.can('user.manage'));
	const runtimes = createQuery(() => ({
		queryKey: runtimeKeys.all,
		queryFn: runtimesApi.list,
		enabled: canManage
	}));

	const toRecord = (p: Project) => Object.fromEntries(p.runtimes.map((r) => [r.name, r.spec]));
	// The draft starts from the project as loaded; saving takes the server's values.
	let saved = $state<Record<string, string>>(untrack(() => toRecord(project)));
	let draft = $state<Record<string, string>>(untrack(() => toRecord(project)));
	const changed = $derived(
		Object.keys(draft).filter((k) => (draft[k] ?? '').trim() !== (saved[k] ?? ''))
	);
	$effect(() => {
		dirty = changed.length > 0;
	});

	let saving = $state(false);
	let errors = $state<Record<string, string>>({});
	let failed = $state('');
	async function save() {
		errors = {};
		failed = '';
		const body: Record<string, string> = {};
		for (const k of changed) {
			const v = (draft[k] ?? '').trim();
			if (!v) {
				errors = { [k]: `Enter a version, or pick Auto or Off.` };
				return;
			}
			body[k] = v;
		}
		saving = true;
		try {
			const p = await projectsApi.update(project.id, { runtimes: body });
			queryClient.setQueryData(projectKeys.detail(p.id), p);
			saved = toRecord(p);
			draft = toRecord(p);
			toast.success('Settings saved');
		} catch (err) {
			const field = err instanceof ApiError ? err.field?.replace(/^runtimes\./, '') : undefined;
			if (field && field in draft) errors = { [field]: (err as ApiError).message };
			else failed = errorMessage(err);
		} finally {
			saving = false;
		}
	}
</script>

<SettingsCard
	title="Runtimes"
	description="The Node.js and PHP versions on the deploy script's PATH, with npm, corepack and Composer."
	{dirty}
	{saving}
	error={failed}
	onsave={save}
>
	{#snippet note()}
		Versions are installed on the first deployment that needs them.
	{/snippet}
	<div class="flex flex-col divide-y">
		{#each project.runtimes as r (r.name)}
			<div class="py-4 first:pt-0 last:pb-0">
				<RuntimeSettingRow
					name={r.name}
					title={runtimes.data?.find((s) => s.name === r.name)?.title ??
						runtimeTitles[r.name] ??
						r.name}
					bind:value={draft[r.name]!}
					used={r.used}
					status={runtimes.data?.find((s) => s.name === r.name)}
					error={errors[r.name]}
					{canManage}
				/>
			</div>
		{/each}
	</div>
</SettingsCard>

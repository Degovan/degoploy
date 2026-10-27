<script lang="ts">
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { toast } from 'svelte-sonner';
	import { errorMessage } from '#lib/api/client.js';
	import { projectKeys, projectsApi } from '#lib/api/projects.js';
	import ProjectForm, {
		projectToForm,
		type ProjectFormValues
	} from '#lib/components/project-form.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import * as Card from '#lib/components/ui/card/index.js';
	import * as AlertDialog from '#lib/components/ui/alert-dialog/index.js';
	import { buttonVariants } from '#lib/components/ui/button/index.js';
	import LoaderIcon from '@lucide/svelte/icons/loader-circle';
	import Trash2Icon from '@lucide/svelte/icons/trash-2';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	const queryClient = useQueryClient();

	const project = createQuery(() => ({
		queryKey: projectKeys.detail(data.id),
		queryFn: () => projectsApi.get(data.id)
	}));

	async function save(values: ProjectFormValues) {
		// eslint-disable-next-line @typescript-eslint/no-unused-vars
		const { slug, default_branch, ...changes } = values;
		const p = await projectsApi.update(data.id, changes);
		queryClient.setQueryData(projectKeys.detail(p.id), p);
		await queryClient.invalidateQueries({ queryKey: projectKeys.all, exact: true });
		await queryClient.invalidateQueries({ queryKey: projectKeys.script(p.id) });
		toast.success('Project saved');
	}

	let deleteOpen = $state(false);
	let confirmSlug = $state('');
	let deleting = $state(false);

	async function remove() {
		deleting = true;
		try {
			await projectsApi.remove(data.id);
			queryClient.removeQueries({ queryKey: projectKeys.detail(data.id) });
			await queryClient.invalidateQueries({ queryKey: projectKeys.all, exact: true });
			toast.success('Project deleted');
			await goto(resolve('/(app)/projects'));
		} catch (err) {
			toast.error(errorMessage(err));
		} finally {
			deleting = false;
		}
	}
</script>

{#if project.data}
	{@const p = project.data}
	{#key p.updated_at}
		<ProjectForm
			initial={projectToForm(p)}
			mode="edit"
			title="Project settings"
			description="Repository, working directory and how the deploy script runs."
			submitLabel="Save changes"
			onsubmit={save}
		/>
	{/key}

	<Card.Root class="border-destructive/40">
		<Card.Header>
			<Card.Title>Delete project</Card.Title>
			<Card.Description>
				Removes the project, its deploy key, managed script, revisions and deployment history. Files
				in <code class="text-xs">{p.work_dir}</code> are left untouched.
			</Card.Description>
		</Card.Header>
		<Card.Footer>
			<AlertDialog.Root bind:open={deleteOpen} onOpenChange={() => (confirmSlug = '')}>
				<AlertDialog.Trigger class={buttonVariants({ variant: 'destructive' })}>
					<Trash2Icon />
					Delete project
				</AlertDialog.Trigger>
				<AlertDialog.Content>
					<AlertDialog.Header>
						<AlertDialog.Title>Delete "{p.name}"?</AlertDialog.Title>
						<AlertDialog.Description>
							This cannot be undone. Type <code class="font-semibold">{p.slug}</code> to confirm.
						</AlertDialog.Description>
					</AlertDialog.Header>
					<div class="flex flex-col gap-2">
						<Label for="confirm-slug" class="sr-only">Project slug</Label>
						<Input
							id="confirm-slug"
							bind:value={confirmSlug}
							autocomplete="off"
							class="font-mono"
						/>
					</div>
					<AlertDialog.Footer>
						<AlertDialog.Cancel disabled={deleting}>Cancel</AlertDialog.Cancel>
						<Button
							variant="destructive"
							disabled={confirmSlug !== p.slug || deleting}
							onclick={remove}
						>
							{#if deleting}<LoaderIcon class="animate-spin" />{/if}
							Delete project
						</Button>
					</AlertDialog.Footer>
				</AlertDialog.Content>
			</AlertDialog.Root>
		</Card.Footer>
	</Card.Root>
{/if}

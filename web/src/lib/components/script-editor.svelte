<script lang="ts">
	import { onMount } from 'svelte';
	import { basicSetup } from 'codemirror';
	import { EditorView, keymap } from '@codemirror/view';
	import { Compartment, EditorState } from '@codemirror/state';
	import { StreamLanguage } from '@codemirror/language';
	import { shell } from '@codemirror/legacy-modes/mode/shell';
	import { indentWithTab } from '@codemirror/commands';

	let {
		value = $bindable(''),
		readonly = false,
		onsave
	}: { value?: string; readonly?: boolean; onsave?: () => void } = $props();

	let host: HTMLDivElement;
	let view: EditorView | undefined;
	const editable = new Compartment();

	// Colours come from the app's CSS variables so light/dark themes just work.
	const theme = EditorView.theme({
		'&': {
			fontSize: '13px',
			backgroundColor: 'var(--background)',
			color: 'var(--foreground)',
			height: '100%'
		},
		'.cm-scroller': { fontFamily: 'var(--font-mono, ui-monospace, monospace)', lineHeight: '1.55' },
		'.cm-content': { caretColor: 'var(--foreground)' },
		'.cm-gutters': {
			backgroundColor: 'var(--muted)',
			color: 'var(--muted-foreground)',
			borderRight: '1px solid var(--border)'
		},
		'.cm-activeLine, .cm-activeLineGutter': {
			backgroundColor: 'color-mix(in oklch, var(--muted) 60%, transparent)'
		},
		'&.cm-focused': { outline: 'none' },
		'&.cm-focused .cm-cursor': { borderLeftColor: 'var(--foreground)' },
		'.cm-selectionBackground, &.cm-focused .cm-selectionBackground, ::selection': {
			backgroundColor: 'color-mix(in oklch, var(--ring) 35%, transparent) !important'
		}
	});

	onMount(() => {
		view = new EditorView({
			parent: host,
			state: EditorState.create({
				doc: value,
				extensions: [
					basicSetup,
					StreamLanguage.define(shell),
					keymap.of([
						indentWithTab,
						{
							key: 'Mod-s',
							preventDefault: true,
							run: () => {
								onsave?.();
								return true;
							}
						}
					]),
					theme,
					editable.of([EditorState.readOnly.of(readonly), EditorView.editable.of(!readonly)]),
					EditorView.updateListener.of((u) => {
						if (u.docChanged) value = u.state.doc.toString();
					})
				]
			})
		});
		return () => view?.destroy();
	});

	// Push external value changes (e.g. restoring a revision) into the editor.
	$effect(() => {
		const next = value;
		if (view && next !== view.state.doc.toString()) {
			view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: next } });
		}
	});

	$effect(() => {
		view?.dispatch({
			effects: editable.reconfigure([
				EditorState.readOnly.of(readonly),
				EditorView.editable.of(!readonly)
			])
		});
	});
</script>

<div bind:this={host} class="h-full min-h-80 overflow-hidden rounded-md border"></div>

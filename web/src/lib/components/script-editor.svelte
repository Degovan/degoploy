<script lang="ts">
	import { onMount } from 'svelte';
	import { basicSetup } from 'codemirror';
	import { EditorView, keymap } from '@codemirror/view';
	import { Compartment, EditorState } from '@codemirror/state';
	import { HighlightStyle, StreamLanguage, syntaxHighlighting } from '@codemirror/language';
	import { shell } from '@codemirror/legacy-modes/mode/shell';
	import { indentWithTab } from '@codemirror/commands';
	import { tags as t } from '@lezer/highlight';
	import { cn } from '#lib/utils.js';

	let {
		value = $bindable(''),
		// eslint-disable-next-line no-useless-assignment -- written for the parent to bind
		cursor = $bindable({ line: 1, col: 1 }),
		readonly = false,
		onsave,
		class: className
	}: {
		value?: string;
		/** 1-based position of the main cursor. */
		cursor?: { line: number; col: number };
		readonly?: boolean;
		onsave?: () => void;
		class?: string;
	} = $props();

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
		'.cm-scroller': {
			fontFamily: 'var(--font-mono, ui-monospace, monospace)',
			lineHeight: '24px',
			scrollbarWidth: 'thin'
		},
		'.cm-content': { padding: '10px 0 14px', caretColor: 'var(--foreground)' },
		'.cm-line': { padding: '0 20px' },
		'.cm-gutters': {
			backgroundColor: 'var(--card)',
			color: 'color-mix(in oklch, var(--muted-foreground) 80%, transparent)',
			border: 'none'
		},
		'.cm-lineNumbers .cm-gutterElement': {
			boxSizing: 'border-box',
			minWidth: '52px',
			padding: '0 12px 0 8px',
			fontSize: '12px'
		},
		'.cm-gutter.cm-foldGutter': { display: 'none !important' },
		'.cm-activeLine, .cm-activeLineGutter': {
			backgroundColor: 'color-mix(in oklch, var(--foreground) 3%, transparent)'
		},
		'.cm-activeLineGutter': { color: 'var(--muted-foreground)' },
		'&.cm-focused': { outline: 'none' },
		'.cm-cursor, &.cm-focused .cm-cursor': {
			borderLeftColor: 'var(--foreground)',
			borderLeftWidth: '2px'
		},
		'.cm-selectionBackground, &.cm-focused .cm-selectionBackground, ::selection': {
			backgroundColor: 'color-mix(in oklch, var(--ring) 35%, transparent) !important'
		},
		'.cm-selectionMatch': {
			backgroundColor: 'color-mix(in oklch, var(--ring) 20%, transparent)'
		},
		'.cm-matchingBracket, &.cm-focused .cm-matchingBracket': {
			backgroundColor: 'color-mix(in oklch, var(--foreground) 12%, transparent)',
			outline: 'none'
		}
	});

	// Token colours of the shell mode: keywords and common commands, strings,
	// $variables, flags and comments.
	const highlight = HighlightStyle.define([
		{ tag: [t.keyword, t.standard(t.variableName)], color: 'var(--code-keyword)' },
		{ tag: [t.string, t.special(t.string)], color: 'var(--code-string)' },
		{ tag: t.definition(t.variableName), color: 'var(--code-variable)' },
		{ tag: [t.number, t.atom, t.bool], color: 'var(--code-number)' },
		{ tag: [t.attributeName, t.operator], color: 'var(--muted-foreground)' },
		{
			tag: [t.comment, t.meta],
			color: 'color-mix(in oklch, var(--muted-foreground) 80%, transparent)'
		}
	]);

	onMount(() => {
		view = new EditorView({
			parent: host,
			state: EditorState.create({
				doc: value,
				extensions: [
					basicSetup,
					StreamLanguage.define(shell),
					syntaxHighlighting(highlight),
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
						if (u.docChanged || u.selectionSet) {
							const head = u.state.selection.main.head;
							const line = u.state.doc.lineAt(head);
							cursor = { line: line.number, col: head - line.from + 1 };
						}
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

<div bind:this={host} class={cn('h-full min-h-0 overflow-hidden', className)}></div>

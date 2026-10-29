// Mermaid diagrams in the docs, themed to match the app. Mermaid parses only
// hex colors, so the palettes mirror the oklch tokens in layout.css.

const palettes = {
	dark: {
		canvas: '#121212',
		node: '#1a1a1a',
		nodeBorder: '#333333',
		text: '#ededed',
		muted: '#a1a1a1',
		line: '#525252',
		// Tinted node kinds: [fill, border, text].
		start: ['#0d1b2e', '#1d4778', '#7cbcff'],
		ok: ['#0c2620', '#1b5e4f', '#50e3c2'],
		bad: ['#2b1214', '#73282e', '#ff7a80'],
		warn: ['#2a210f', '#6b5221', '#f5b544']
	},
	light: {
		canvas: '#ffffff',
		node: '#fafafa',
		nodeBorder: '#e4e4e7',
		text: '#18181b',
		muted: '#71717a',
		line: '#a1a1aa',
		start: ['#eff6ff', '#bfdbfe', '#1d4ed8'],
		ok: ['#ecfdf5', '#a7f3d0', '#047857'],
		bad: ['#fef2f2', '#fecaca', '#b91c1c'],
		warn: ['#fffbeb', '#fde68a', '#b45309']
	}
} as const;

export function diagramConfig(dark: boolean) {
	const p = dark ? palettes.dark : palettes.light;
	return {
		startOnLoad: false,
		securityLevel: 'strict' as const,
		theme: 'base' as const,
		look: 'neo' as const,
		fontFamily: "'Geist Variable', ui-sans-serif, system-ui, sans-serif",
		fontSize: 14,
		themeVariables: {
			darkMode: dark,
			background: p.canvas,
			fontFamily: "'Geist Variable', ui-sans-serif, system-ui, sans-serif",
			fontSize: '14px',
			primaryColor: p.node,
			primaryBorderColor: p.nodeBorder,
			primaryTextColor: p.text,
			secondaryColor: p.node,
			secondaryBorderColor: p.nodeBorder,
			secondaryTextColor: p.text,
			tertiaryColor: p.canvas,
			tertiaryBorderColor: p.nodeBorder,
			tertiaryTextColor: p.text,
			mainBkg: p.node,
			nodeBorder: p.nodeBorder,
			nodeTextColor: p.text,
			textColor: p.muted,
			lineColor: p.line,
			edgeLabelBackground: p.canvas,
			clusterBkg: p.canvas,
			clusterBorder: p.nodeBorder,
			// Sequence diagrams.
			actorBkg: p.node,
			actorBorder: p.nodeBorder,
			actorTextColor: p.text,
			actorLineColor: p.nodeBorder,
			signalColor: p.line,
			signalTextColor: p.muted,
			labelBoxBkgColor: p.node,
			labelBoxBorderColor: p.nodeBorder,
			labelTextColor: p.text,
			loopTextColor: p.muted,
			noteBkgColor: p.node,
			noteBorderColor: p.nodeBorder,
			noteTextColor: p.text,
			activationBkgColor: p.node,
			activationBorderColor: p.nodeBorder,
			sequenceNumberColor: p.canvas
		},
		// Flat, rounded nodes and quiet edge labels, like the rest of the UI. The
		// guide tags nodes with "class X start|ok|bad|warn|muted" to tint them.
		themeCSS: `
			.node .label-container, .node rect, .node polygon, .node path, .actor { filter: none !important; }
			.node rect.label-container { rx: 10px; ry: 10px; }
			.node .label-container { stroke-width: 1px; }
			.nodeLabel, .nodeLabel p { color: ${p.text}; font-weight: 500; }
			.edgeLabel, .edgeLabel p, .edgeLabel span { color: ${p.muted}; font-size: 12px; }
			.labelBkg, .edgeLabel rect { background: ${p.canvas} !important; fill: ${p.canvas} !important; opacity: 1; border-radius: 6px; padding: 0 4px; }
			.flowchart-link { stroke-width: 1.5px; }
			.flowchart-link.edge-pattern-dotted { stroke-dasharray: 2 4 !important; stroke-linecap: round; }
			.marker, marker path { fill: ${p.line} !important; stroke: ${p.line} !important; }
			${(['start', 'ok', 'bad', 'warn'] as const)
				.map(
					(k) => `
			.node.${k} .label-container, .node.${k} .label-container path { fill: ${p[k][0]} !important; stroke: ${p[k][1]} !important; }
			.node.${k} .nodeLabel, .node.${k} .nodeLabel p { color: ${p[k][2]}; }`
				)
				.join('')}
			.node.muted .label-container, .node.muted .label-container path { fill: ${p.canvas} !important; stroke: ${p.line} !important; stroke-dasharray: 4 3; }
			.node.muted .nodeLabel, .node.muted .nodeLabel p { color: ${p.muted}; font-weight: 400; }
			.cluster rect { rx: 12px; ry: 12px; fill: transparent !important; stroke: ${p.nodeBorder} !important; stroke-dasharray: 4 4; }
			.cluster-label .nodeLabel, .cluster-label .nodeLabel p { color: ${p.muted}; font-size: 12px; font-weight: 500; letter-spacing: 0.02em; text-transform: uppercase; }
			.messageLine0, .messageLine1 { stroke-width: 1.5px; }
			.messageText { fill: ${p.muted} !important; font-size: 13px; }
			text.actor > tspan { fill: ${p.text} !important; font-weight: 500; }
			rect.actor { rx: 10px; ry: 10px; }
			.sequenceNumber { fill: ${p.canvas} !important; font-weight: 600; }
			#sequencenumber { fill: ${p.text} !important; }
		`,
		flowchart: {
			useMaxWidth: false,
			curve: 'basis' as const,
			nodeSpacing: 28,
			rankSpacing: 40,
			padding: 12,
			wrappingWidth: 160
		},
		sequence: {
			useMaxWidth: false,
			mirrorActors: false,
			showSequenceNumbers: true,
			actorMargin: 96,
			messageMargin: 32,
			boxMargin: 8
		}
	};
}

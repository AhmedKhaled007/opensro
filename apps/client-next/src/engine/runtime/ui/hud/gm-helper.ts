/*
===========================================================================

gm-helper.ts - the GM console's inline codename list

DELIBERATE ADDITION, NOT NATIVE: retail's console (Console_OnInputKey,
50F260) takes /MAKEITEM and /LOADMONSTER only from what the GM types. This
owner filters the published codename catalogues as the line changes, keeps
the highlighted row, and turns Tab or Enter into a completed command. The
console keeps its native 600x112 panel otherwise; a catalogue grows it so
the list shows the same rows the native composer accepts.

===========================================================================
*/
import type { GmCatalog } from "@/engine/foundation/gameplay/gm-command";

// The list's row pitch is the console's own (13 px). Twenty-two rows plus a
// 20 px tab header, a 4 px gap and the prompt row keep the whole panel
// inside a 414 px viewport.
export const GM_HELPER_ROWS = 22;
export const GM_HELPER_ROW_HEIGHT = 13;
export const GM_HELPER_HEIGHT = GM_HELPER_ROWS * GM_HELPER_ROW_HEIGHT + 48;

const GM_HELPER_COMMANDS = [ "/MAKEITEM", "/LOADMONSTER" ] as const;
type GmHelperCommand = (typeof GM_HELPER_COMMANDS)[number];

/*
================
GmHelperTab

The three catalogues the browser cycles through. Uniques are the monster
rows whose characterdata monster-type byte is 3.
================
*/
export type GmHelperTab = "items" | "monsters" | "uniques";
const GM_HELPER_TABS = [ "items", "monsters", "uniques" ] as const;

/*
================
GmHelperCounts
================
*/
export interface GmHelperCounts {
	readonly items: number;
	readonly monsters: number;
	readonly uniques: number;
}

/*
================
GmHelperRow

One list line: the codename Tab inserts and the display name beside it.
================
*/
export interface GmHelperRow {
	readonly codename: string;
	readonly name: string;
	// The composer's default trailing argument: a stackable item's count, an
	// equipment row's plus level (0, so the row stays unenhanced), or a
	// monster's count.
	readonly amount: number;
}

/*
================
GmHelperState

The draw inputs. rows is the visible window beginning at first; selected is
an index into the whole match list.
================
*/
export interface GmHelperState {
	readonly active: boolean;
	readonly selected: number;
	readonly first: number;
	readonly total: number;
	readonly rows: readonly GmHelperRow[];
}

/*
================
createGmHelper

The helper owns no catalogues of its own: the entry bootstrap's public
reference slices arrive through setCatalog. State changes only here, so the
console never keeps a second copy of the selection window.
================
*/
export function createGmHelper() {
	let catalog: GmCatalog | null = null;
	let command: GmHelperCommand | null = null;
	// Port-only: F3 opens a catalogue without typing a slash command. The
	// prompt's text then filters that catalogue; Escape or a second F3 closes it.
	let browse: GmHelperTab | null = null;
	let query = "", dismissed = "", lastLine = "", lastVisible = GM_HELPER_ROWS;
	let matches: readonly GmHelperRow[] = [], selected = 0, first = 0, visible = GM_HELPER_ROWS;
	let items: readonly GmHelperRow[] = [], monsters: readonly GmHelperRow[] = [], uniques: readonly GmHelperRow[] = [];
	let amounts = new Map<string, number>();

	/*
	================
	commandForTab
	================
	*/
	function commandForTab( tab: GmHelperTab ): GmHelperCommand {
		return tab === "items" ? "/MAKEITEM" : "/LOADMONSTER";
	}

	/*
	================
	tabForCommand
	================
	*/
	function tabForCommand( name: GmHelperCommand ): GmHelperTab {
		return name === "/MAKEITEM" ? "items" : "monsters";
	}

	/*
	================
	sourceFor
	================
	*/
	function sourceFor( tab: GmHelperTab ): readonly GmHelperRow[] {
		return tab === "items" ? items : tab === "uniques" ? uniques : monsters;
	}

	/*
	================
	deactivate
	================
	*/
	function deactivate() {
		command = null;
		query = "";
		matches = [];
		selected = 0;
		first = 0;
	}

	/*
	================
	clampWindow

	The visible window follows the selection: one row past the top scrolls
	up, one past the bottom scrolls down, and the last row never leaves a
	blank row below the list.
	================
	*/
	function clampWindow() {
		const count = Math.max( 1, Math.min( visible, GM_HELPER_ROWS ) );
		if ( selected < first ) first = selected;
		if ( selected >= first + count ) first = selected - count + 1;
		first = Math.max( 0, Math.min( first, Math.max( 0, matches.length - count ) ) );
	}

	/*
	================
	state
	================
	*/
	function state(): GmHelperState {
		if ( !command ) return { active: false, selected: 0, first: 0, total: 0, rows: [] };
		const count = Math.max( 1, Math.min( visible, GM_HELPER_ROWS ) );
		clampWindow();
		return {
			active: true,
			selected,
			first,
			total: matches.length,
			rows: matches.slice( first, first + count )
		};
	}

	/*
	================
	moveSelection
	================
	*/
	function moveSelection( delta: number ) {
		if ( !command || !matches.length ) return;
		selected = Math.max( 0, Math.min( matches.length - 1, selected + delta ) );
	}

	/*
	================
	commandFor

	Only the first argument is a codename. A second word is the amount, the
	count or the monster type, so the list closes rather than pretending the
	typed number is a codename prefix.
	================
	*/
	function commandFor( line: string ): { command: GmHelperCommand; query: string; } | null {
		for ( const name of GM_HELPER_COMMANDS ) {
			if ( line === name ) return { command: name, query: "" };
			if ( !line.startsWith( name + " " ) ) continue;
			const rest = line.slice( name.length + 1 );
			if ( /\s/.test( rest ) ) return null;
			return { command: name, query: rest };
		}
		return null;
	}

	return {
		/*
		================
		setCatalog

		The entry bootstrap owns the catalogues; a new world replaces them.
		================
		*/
		setCatalog( value: GmCatalog | null ) {
			if ( value === catalog ) return;
			catalog = value;
			// The server's item rows decide the default trailing argument once:
			// a stackable row (type1 = 3, the composer's 0x60 mask) takes a
			// count of 1, and an equipment row takes plus 0 so Enter never
			// silently creates a +1 copy.
			items = value ?
				value.items.map( row => ({
					codename: row.codename,
					name: row.name,
					amount: (row.typeFlags & 0x60) === 0x60 ? 1 : 0
				}) ) :
				[];
			monsters = value ?
				value.monsters.map( row => ({ codename: row.codename, name: row.name, amount: 1 }) ) :
				[];
			// characterdata column 15: 3 marks the 19 unique monsters.
			uniques = value ?
				value.monsters.filter( row => row.monsterType === 3 ).map( row => ({
					codename: row.codename,
					name: row.name,
					amount: 1
				}) ) :
				[];
			amounts = new Map( [
				...items.map( row => [ row.codename, row.amount ] as const ),
				...monsters.map( row => [ row.codename, row.amount ] as const )
			] );
			// Keep an F3 browse that opened before this catalogue arrived: it
			// filters the rows as soon as they exist.
			dismissed = "";
			lastLine = "";
			deactivate();
		},
		/*
		================
		open

		F3 opens a catalogue with an empty prompt. A bare prompt text filters
		the open catalogue; a slash command still takes the normal path.
		================
		*/
		open( tab: GmHelperTab = "items" ) {
			browse = tab;
			dismissed = "";
			lastLine = "\0";
			lastVisible = -1;
			deactivate();
		},
		/*
		================
		selectTab

		A header tab click picks one catalogue and starts its filter over.
		================
		*/
		selectTab( tab: GmHelperTab ) {
			browse = tab;
			dismissed = "";
			lastLine = "\0";
			deactivate();
		},
		/*
		================
		cycle

		Left and Right move through Items, Monsters and Uniques while the
		pointer is not editing filter text.
		================
		*/
		cycle( delta: number ) {
			const current = browse ?? "items";
			const index = GM_HELPER_TABS.indexOf( current );
			const next = GM_HELPER_TABS[(index + delta + GM_HELPER_TABS.length) % GM_HELPER_TABS.length]!;
			browse = next;
			dismissed = "";
			lastLine = "\0";
			deactivate();
		},
		/*
		================
		tab
		================
		*/
		tab(): GmHelperTab {
			return browse ?? "items";
		},
		/*
		================
		counts
		================
		*/
		counts(): GmHelperCounts {
			return { items: items.length, monsters: monsters.length, uniques: uniques.length };
		},
		/*
		================
		closeBrowse

		F3 on an open catalogue closes the list and leaves the console open.
		================
		*/
		closeBrowse() {
			browse = null;
			dismissed = "";
			lastLine = "\0";
			deactivate();
		},
		/*
		================
		browsing
		================
		*/
		browsing(): boolean {
			return browse !== null;
		},
		/*
		================
		observe

		Recompute the list when the line, the catalogue or the visible
		height changes. The returned state is the draw input.
		================
		*/
		observe( line: string, rowsVisible = GM_HELPER_ROWS ): GmHelperState {
			if ( line === lastLine && rowsVisible === lastVisible ) return state();
			lastLine = line;
			lastVisible = rowsVisible;
			visible = rowsVisible;
			if ( line !== dismissed ) dismissed = "";
			if ( !catalog || (dismissed !== "" && line === dismissed) ) {
				deactivate();
				return state();
			}
			const parsed = commandFor( line );
			let tab: GmHelperTab | null = null, text = "";
			if ( browse ) {
				// F3's catalogue: the prompt filters it until the GM types a
				// slash command. A typed amount or another command closes it.
				if ( line.startsWith( "/" ) ) {
					if ( !parsed ) {
						// Another command, or a catalog command with its amount
						// already typed: leave the browser for a fresh F3.
						browse = null;
						deactivate();
						return state();
					}
					tab = tabForCommand( parsed.command );
					text = parsed.query;
				} else {
					tab = browse;
					text = line;
				}
			} else if ( parsed ) {
				tab = tabForCommand( parsed.command );
				text = parsed.query;
			} else {
				deactivate();
				return state();
			}
			browse = tab;
			const needle = text.toLowerCase();
			const source = sourceFor( tab );
			const found = needle ?
				source.filter( row =>
					row.codename.toLowerCase().includes( needle ) || row.name.toLowerCase().includes( needle )
				) :
				source;
			if ( text !== query ) {
				selected = 0;
				first = 0;
			}
			command = commandForTab( tab );
			query = text;
			matches = found;
			if ( selected >= matches.length ) selected = Math.max( 0, matches.length - 1 );
			return state();
		},
		/*
		================
		active
		================
		*/
		active(): boolean {
			return command !== null;
		},
		/*
		================
		move
		================
		*/
		move( delta: number ) {
			moveSelection( delta );
		},
		/*
		================
		page
		================
		*/
		page( delta: number ) {
			moveSelection( delta * Math.max( 1, Math.min( visible, GM_HELPER_ROWS ) - 1 ) );
		},
		/*
		================
		select
		================
		*/
		select( index: number ) {
			if ( !command || !matches.length ) return;
			selected = Math.max( 0, Math.min( matches.length - 1, index ) );
		},
		/*
		================
		complete

		Tab: replace the typed argument with the highlighted codename and
		leave the prompt editable for the amount or count.
		================
		*/
		complete( line: string ): string {
			if ( !command || !matches.length ) return line;
			const row = matches[selected]!;
			const completed = command + " " + row.codename + " ";
			browse = null;
			dismissed = completed;
			lastLine = completed;
			deactivate();
			return completed;
		},
		/*
		================
		run

		Enter on an open list runs the highlighted row with the composer's
		default amount (1). A typed amount keeps the native path: the list is
		closed and the line is sent unchanged.
		================
		*/
		run( line: string ): string {
			if ( command && matches.length ) {
				const row = matches[selected]!;
				const completed = command + " " + row.codename + " " + row.amount;
				browse = null;
				dismissed = completed;
				lastLine = completed;
				deactivate();
				return completed;
			}
			// Tab leaves the codename and a trailing space for the amount. If
			// the GM presses Enter there, add the composer's default amount.
			// A second word (amount, count or type) is left exactly as typed.
			const named = /^\/(MAKEITEM|LOADMONSTER) (\S+)\s*$/.exec( line );
			if ( !named ) return line;
			const amount = amounts.get( named[2]! ) ?? (named[1] === "LOADMONSTER" ? 1 : 0);
			return "/" + named[1] + " " + named[2] + " " + amount;
		},
		/*
		================
		dismiss

		Escape closes the list but keeps the prompt and its text. The exact
		line stays closed until the GM edits it.
		================
		*/
		dismiss( line: string ) {
			browse = null;
			dismissed = line;
			lastLine = line;
			deactivate();
		},
		/*
		================
		reset

		World exit and console close discard the selection but keep the
		catalogue until setCatalog replaces it.
		================
		*/
		reset() {
			browse = null;
			dismissed = "";
			lastLine = "";
			deactivate();
		}
	};
}

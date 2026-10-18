/*
===========================================================================

gm-helper.test.mjs - the GM console's inline codename list

The helper is pure list state: it filters the bootstrap catalogues, owns the
selection window, and completes a command line. The console only draws it.

===========================================================================
*/
import { test } from "node:test";
import assert from "node:assert/strict";
import "../helpers/native-source-loader.mjs";

const { gmCatalog, gmItemReferences } = await import(
	"../../src/engine/foundation/gameplay/gm-command.ts"
);
const { GM_HELPER_ROWS, createGmHelper } = await import( "../../src/engine/runtime/ui/hud/gm-helper.ts" );

const items = [
	{
		refObjId: 107,
		codename: "ITEM_CH_BLADE_01_A",
		name: "Copper Blade",
		typeFlags: 42,
		maxStack: 1
	},
	{
		refObjId: 8,
		codename: "ITEM_ETC_HP_POTION_05",
		name: "HP recovery potion (X-large)",
		// type1 = 3 (the composer's stackable 0x60 mask).
		typeFlags: 6380,
		maxStack: 500
	}
];
const objects = [
	{
		refObjId: 2001,
		codename: "MOB_CH_MANGNYANG",
		kind: "monster",
		name: "Mangyang",
		level: 1,
		monsterType: 0
	},
	{
		refObjId: 2002,
		codename: "MOB_CH_WHITETIGER",
		kind: "monster",
		name: "White Tiger",
		level: 30,
		monsterType: 1
	},
	{
		refObjId: 2003,
		codename: "MOB_CH_TIGERWOMAN",
		kind: "monster",
		name: "Tiger Woman",
		level: 40,
		monsterType: 3
	},
	{ refObjId: 3001, codename: "NPC_CH_MERCHANT", kind: "npc", name: "Merchant", level: 1 }
];

/*
================
catalog
================
*/
function catalog() {
	return gmCatalog( { itemCommandReferences: items, refObjSnapshot: objects } );
}

/*
================
test("the catalog carries codenames, display names and monsters only")
================
*/
test("the catalog carries codenames, display names and monsters only", () => {
	const value = catalog();
	assert.deepEqual( value.items.map( row => [ row.codename, row.name ] ), [
		[ "ITEM_CH_BLADE_01_A", "Copper Blade" ],
		[ "ITEM_ETC_HP_POTION_05", "HP recovery potion (X-large)" ]
	] );
	assert.deepEqual( value.monsters.map( row => [ row.codename, row.name ] ), [
		[ "MOB_CH_MANGNYANG", "Mangyang" ],
		[ "MOB_CH_TIGERWOMAN", "Tiger Woman" ],
		[ "MOB_CH_WHITETIGER", "White Tiger" ]
	] );
});

/*
================
test("a missing item name falls back to the codename")
================
*/
test("a missing item name falls back to the codename", () => {
	const rows = gmItemReferences( {
		itemCommandReferences: [ { refObjId: 107, codename: "ITEM_CH_BLADE_01_A", typeFlags: 42, maxStack: 1 } ]
	} );
	assert.equal( rows.get( "ITEM_CH_BLADE_01_A" )?.name, "ITEM_CH_BLADE_01_A" );
});

/*
================
test("/MAKEITEM filters codenames and display names")
================
*/
test("/MAKEITEM filters codenames and display names", () => {
	const helper = createGmHelper();
	helper.setCatalog( catalog() );
	let state = helper.observe( "/MAKEITEM " );
	assert.equal( state.active, true );
	assert.equal( state.total, 2 );
	state = helper.observe( "/MAKEITEM blade" );
	assert.equal( state.total, 1 );
	assert.equal( state.rows[0].codename, "ITEM_CH_BLADE_01_A" );
	state = helper.observe( "/MAKEITEM COPPER" );
	assert.equal( state.rows[0].codename, "ITEM_CH_BLADE_01_A" );
	state = helper.observe( "/MAKEITEM potion" );
	assert.equal( state.rows[0].codename, "ITEM_ETC_HP_POTION_05" );
});

/*
================
test("F3 opens a catalogue with an empty prompt and filters from it")
================
*/
test("F3 opens a catalogue with an empty prompt and filters from it", () => {
	const helper = createGmHelper();
	helper.setCatalog( catalog() );
	assert.deepEqual( helper.counts(), { items: 2, monsters: 3, uniques: 1 } );
	helper.open( "items" );
	let state = helper.observe( "" );
	assert.equal( state.active, true );
	assert.equal( state.total, 2 );
	state = helper.observe( "copper" );
	assert.equal( state.total, 1 );
	assert.equal( state.rows[0].codename, "ITEM_CH_BLADE_01_A" );
	helper.closeBrowse();
	assert.equal( helper.browsing(), false );
	assert.equal( helper.observe( "" ).active, false );
	helper.open( "monsters" );
	assert.equal( helper.observe( "" ).total, 3 );
	helper.cycle( 1 );
	assert.equal( helper.tab(), "uniques" );
	state = helper.observe( "" );
	assert.equal( state.total, 1 );
	assert.equal( state.rows[0].codename, "MOB_CH_TIGERWOMAN" );
	// An empty prompt has no caret: Right/Left cycle the tab, and the typed
	// command still decides the catalogue.
	helper.observe( "/LOADMONSTER mang" );
	assert.equal( helper.tab(), "monsters" );
	assert.equal( helper.observe( "/LOADMONSTER mang" ).total, 1 );
});

/*
================
test("/LOADMONSTER lists monsters and ignores NPC rows")
================
*/
test("/LOADMONSTER lists monsters and ignores NPC rows", () => {
	const helper = createGmHelper();
	helper.setCatalog( catalog() );
	let state = helper.observe( "/LOADMONSTER " );
	assert.equal( state.total, 3 );
	assert.equal( state.rows.some( row => row.codename === "NPC_CH_MERCHANT" ), false );
	state = helper.observe( "/LOADMONSTER whit" );
	assert.equal( state.rows[0].codename, "MOB_CH_WHITETIGER" );
});

/*
================
test("a second word closes the list")
================
*/
test("a second word closes the list", () => {
	const helper = createGmHelper();
	helper.setCatalog( catalog() );
	helper.observe( "/MAKEITEM blade" );
	helper.move( 0 );
	assert.equal( helper.observe( "/MAKEITEM ITEM_CH_BLADE_01_A 5" ).active, false );
	assert.equal( helper.active(), false );
	assert.equal( helper.observe( "/FINDUSER Remo" ).active, false );
});

/*
================
test("move and page keep the selection inside the matched rows")
================
*/
test("move and page keep the selection inside the matched rows", () => {
	const helper = createGmHelper();
	helper.setCatalog( catalog() );
	let state = helper.observe( "/MAKEITEM " );
	assert.equal( state.selected, 0 );
	helper.move( 5 );
	state = helper.observe( "/MAKEITEM " );
	assert.equal( state.selected, 1 );
	helper.move( -5 );
	state = helper.observe( "/MAKEITEM " );
	assert.equal( state.selected, 0 );
	helper.page( 1 );
	state = helper.observe( "/MAKEITEM " );
	assert.equal( state.selected, 1 );
});

/*
================
test("Tab completes the codename and leaves the amount editable")
================
*/
test("Tab completes the codename and leaves the amount editable", () => {
	const helper = createGmHelper();
	helper.setCatalog( catalog() );
	helper.observe( "/MAKEITEM blade" );
	assert.equal( helper.complete( "/MAKEITEM blade" ), "/MAKEITEM ITEM_CH_BLADE_01_A " );
	assert.equal( helper.active(), false );
	assert.equal( helper.observe( "/MAKEITEM ITEM_CH_BLADE_01_A " ).active, false );
	// Enter after Tab still runs the row. An equipment row keeps plus 0;
	// a stackable row counts 1.
	assert.equal( helper.run( "/MAKEITEM ITEM_CH_BLADE_01_A " ), "/MAKEITEM ITEM_CH_BLADE_01_A 0" );
	assert.equal( helper.run( "/MAKEITEM ITEM_ETC_HP_POTION_05 " ), "/MAKEITEM ITEM_ETC_HP_POTION_05 1" );
});

/*
================
test("Enter runs the highlighted row with the default amount")
================
*/
test("Enter runs the highlighted row with the default amount", () => {
	const helper = createGmHelper();
	helper.setCatalog( catalog() );
	helper.observe( "/MAKEITEM " );
	assert.equal( helper.run( "/MAKEITEM " ), "/MAKEITEM ITEM_CH_BLADE_01_A 0" );
	helper.observe( "/MAKEITEM " );
	helper.move( 1 );
	assert.equal( helper.run( "/MAKEITEM " ), "/MAKEITEM ITEM_ETC_HP_POTION_05 1" );
	helper.observe( "/LOADMONSTER mang" );
	assert.equal( helper.run( "/LOADMONSTER mang" ), "/LOADMONSTER MOB_CH_MANGNYANG 1" );
});

/*
================
test("Escape dismisses the current line until it is edited")
================
*/
test("Escape dismisses the current line until it is edited", () => {
	const helper = createGmHelper();
	helper.setCatalog( catalog() );
	helper.observe( "/MAKEITEM blade" );
	helper.dismiss( "/MAKEITEM blade" );
	assert.equal( helper.observe( "/MAKEITEM blade" ).active, false );
	assert.equal( helper.observe( "/MAKEITEM blad" ).active, true );
});

/*
================
test("a new world replaces the list and clears the selection window")
================
*/
test("a new world replaces the list and clears the selection window", () => {
	const helper = createGmHelper();
	helper.setCatalog( catalog() );
	helper.observe( "/MAKEITEM " );
	helper.move( 1 );
	helper.setCatalog( null );
	assert.equal( helper.observe( "/MAKEITEM " ).active, false );
});

/*
================
test("a typed command does not leave F3 browse mode stuck open")
================
*/
test("a typed command does not leave F3 browse mode stuck open", () => {
	const helper = createGmHelper();
	helper.setCatalog( catalog() );
	helper.observe( "/MAKEITEM blade" );
	assert.equal( helper.browsing(), false );
	assert.equal( helper.observe( "" ).active, false );

	// F3 on Uniques, then a typed /LOADMONSTER shows Monsters; clearing the
	// line returns to the F3 tab instead of losing it.
	helper.open( "uniques" );
	helper.observe( "/LOADMONSTER mang" );
	assert.equal( helper.tab(), "monsters" );
	assert.equal( helper.observe( "" ).total, 1 );
	assert.equal( helper.tab(), "uniques" );
});

/*
================
test("lowercase reference commands match the native wcsicmp")
================
*/
test("lowercase reference commands match the native wcsicmp", () => {
	const helper = createGmHelper();
	helper.setCatalog( catalog() );
	let state = helper.observe( "/makeitem blade" );
	assert.equal( state.active, true );
	assert.equal( state.total, 1 );
	assert.equal( helper.run( "/makeitem ITEM_CH_BLADE_01_A " ), "/MAKEITEM ITEM_CH_BLADE_01_A 0" );
	state = helper.observe( "/loadmonster mang" );
	assert.equal( state.active, true );
	assert.equal( state.total, 1 );
	assert.equal( helper.run( "/loadmonster MOB_CH_MANGNYANG " ), "/LOADMONSTER MOB_CH_MANGNYANG 1" );
});

/*
================
test("the visible window follows the selection through every row")
================
*/
test("the visible window follows the selection through every row", () => {
	const many = Array.from( { length: GM_HELPER_ROWS + 5 }, ( _, i ) => ({
		refObjId: i + 1,
		codename: "ITEM_TEST_" + String( i ).padStart( 3, "0" ),
		name: "Test " + i,
		typeFlags: 27,
		maxStack: 1
	}) );
	const helper = createGmHelper();
	helper.setCatalog( gmCatalog( { itemCommandReferences: many } ) );
	let state = helper.observe( "/MAKEITEM " );
	assert.equal( state.rows.length, GM_HELPER_ROWS );
	assert.equal( state.first, 0 );
	helper.move( GM_HELPER_ROWS + 4 );
	state = helper.observe( "/MAKEITEM " );
	assert.equal( state.selected, GM_HELPER_ROWS + 4 );
	assert.equal( state.first, 5 );
	assert.equal(
		state.rows[state.rows.length - 1]?.codename,
		"ITEM_TEST_" + String( GM_HELPER_ROWS + 4 ).padStart( 3, "0" )
	);
});

/*
================
test("uncatalogued monster commands keep the default count")
================
*/
test("uncatalogued monster commands keep the default count", () => {
	const helper = createGmHelper();
	assert.equal( helper.run( "/loadmonster MOB_UNCACHEABLE " ), "/LOADMONSTER MOB_UNCACHEABLE 1" );
	helper.setCatalog( catalog() );
	assert.equal( helper.run( "/LOADMONSTER MOB_UNCACHEABLE " ), "/LOADMONSTER MOB_UNCACHEABLE 1" );
});

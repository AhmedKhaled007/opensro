/*
===========================================================================

gm-console-input.test.mjs - catalogue shortcuts through the production HUD

Exercise prompt ownership and native hotbar paging through public input
and semantic controls, including a console above another open window.

===========================================================================
*/
import { test } from "node:test";
import assert from "node:assert/strict";
import { uiFixture } from "../helpers/ui-fixture.mjs";

const FRAME_MS = 100;
const WARM_FRAMES = 24;

/*
================
gmFixture
================
*/
function gmFixture( t ) {
	const f = uiFixture();
	t.after( () => f.dispose() );
	Object.assign( f.state.gameplay, {
		eligibility: { gm: true, pcRoomEvent: false },
		gmCatalog: {
			items: [ { codename: "ITEM_CH_BLADE_01_A", name: "Copper Blade", typeFlags: 42 } ],
			monsters: []
		},
		inventorySlotCount: 45,
		equipmentSlotCount: 13
	} );
	let now = 0;
	/** @type {import("../../src/engine/contracts/ui").UiSemantics | null} */
	let result = null;
	/*
	================
	draw
	================
	*/
	function draw() {
		result = f.ui.step( f.state, now += FRAME_MS ) ?? result;
		assert.ok( result, "input publishes semantic controls" );
		return result;
	}
	for ( let frame = 0; frame < WARM_FRAMES; frame++ ) draw();
	return { ...f, draw };
}

/*
================
test("gameplay F3 and Shift+F3 select native hotbar page three")
================
*/
test("gameplay F3 and Shift+F3 select native hotbar page three", t => {
	const f = gmFixture( t );
	for ( const shift of [ false, true ] ) {
		f.ui.event( { kind: "key", code: "F1" } );
		assert.ok( f.draw().controls.some( row => row.id === "hotbar:1" ) );
		f.ui.event( { kind: "key", code: "F3", shift } );
		const result = f.draw();
		assert.ok( result.controls.some( row => row.id === "hotbar:21" ), "page three owns slots 21..30" );
		assert.equal( result.controls.some( row => row.id === "gm-input" ), false );
		assert.ok( f.hasText( "F3" ), "the painted page label is F3" );
	}
});

/*
================
test("F3 dismisses typed suggestions and preserves the prompt")
================
*/
test("F3 dismisses typed suggestions and preserves the prompt", t => {
	const f = gmFixture( t );
	f.ui.event( { kind: "key", code: "Backquote", shift: true } );
	f.draw();
	const line = "/MAKEITEM blade";
	f.ui.event( { kind: "edit", id: "gm-input", value: line, start: line.length, end: line.length, composing: false } );
	assert.ok( f.draw().controls.some( row => row.id === "gm-helper-row:0" ) );
	f.ui.event( { kind: "key", code: "F3" } );
	const result = f.draw();
	assert.equal( result.controls.find( row => row.id === "gm-input" )?.value, line );
	assert.equal( result.controls.some( row => row.id.startsWith( "gm-helper-row:" ) ), false );
});

/*
================
test("focused console F3 toggles above Inventory")
================
*/
test("focused console F3 toggles above Inventory", t => {
	const f = gmFixture( t );
	f.ui.event( { kind: "activate", id: "toggle-window:Inventory" } );
	assert.ok( f.draw().controls.some( row => row.id === "close-window:Inventory" ) );
	f.ui.event( { kind: "key", code: "Backquote", shift: true } );
	f.draw();
	f.ui.event( { kind: "key", code: "F3" } );
	let result = f.draw();
	assert.ok( result.controls.some( row => row.id === "gm-helper-row:0" ) );
	assert.ok( result.controls.some( row => row.id === "close-window:Inventory" ) );
	f.ui.event( { kind: "key", code: "F3" } );
	result = f.draw();
	assert.equal( result.controls.some( row => row.id.startsWith( "gm-helper-row:" ) ), false );
	assert.ok( result.controls.some( row => row.id === "gm-input" ) );
});

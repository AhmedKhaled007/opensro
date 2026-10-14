/*
===========================================================================

npc-interaction-result.test.mjs - shared NPC acknowledgement dispatch

Exercises the server's six-byte special-trade acknowledgement through the
inventory and gameplay owners, including pending avatar and Magic Pang windows.
Malformed packets must fail without changing either window's session.

===========================================================================
*/
import "../helpers/native-source-loader.mjs";
import { test } from "node:test";
import assert from "node:assert/strict";
import { defined } from "../helpers/defined.mjs";

const { createInventory } = await import(
	"../../src/engine/runtime/simulation/worker/session/world/gameplay/inventory/inventory.ts"
);
const { createGameplay } = await import(
	"../../src/engine/runtime/simulation/worker/session/world/gameplay/gameplay.ts"
);
const { createGacha } = await import(
	"../../src/engine/runtime/simulation/worker/session/world/gameplay/inventory/gacha/gacha.ts"
);
const { createMagicOptionGrant } = await import(
	"../../src/engine/runtime/simulation/worker/session/world/gameplay/inventory/magic-option/magic-option.ts"
);

const OP_NPC_ACK = 0xb338;
const OP_SELECT_ACK = 0xb45a;
const NPC_GID = 17;
const GACHA_NPC_REF = 9251;
const SPECIAL_TRADE_MASK = 0x800;
const GACHA_MASK = 0x10000;
const AVATAR_MASK = 0x80000000;
// npcaction.go appends this mode byte to EncodeNpcInteractionAck(0x800).
const SPECIAL_TRADE_ACK = Uint8Array.of( 1, 0, 8, 0, 0, 0 );
const WINDOWS = /** @type {const} */ ([
	{ key: "gacha", command: "gacha-open", mask: GACHA_MASK, create: createGacha },
	{ key: "magicOption", command: "magic-option-open", mask: AVATAR_MASK, create: createMagicOptionGrant }
]);

/*
================
serviceAck

Ordinary services carry no mode byte.
================
*/
function serviceAck( mask ) {
	const payload = new Uint8Array( 5 );
	payload[0] = 1;
	new DataView( payload.buffer ).setUint32( 1, mask, true );
	return payload;
}

/*
================
selectedGame

Use the public selection and acknowledgement path before opening a window.
================
*/
function selectedGame( capabilities ) {
	const sent = [], game = createGameplay( frame => sent.push( frame ) );
	const npc = /** @type {const} */ ({
		gid: NPC_GID,
		refObjId: GACHA_NPC_REF,
		kind: "npc",
		name: "NPC service fixture",
		regionId: 1,
		x: 0,
		y: 0,
		z: 0,
		heading: 0
	});
	game.seed( { ...npc, gid: 1 } );
	game.command( { kind: "select", gid: NPC_GID }, 0, npc );
	const payload = new Uint8Array( 11 ), view = new DataView( payload.buffer );
	payload[0] = 1;
	view.setUint32( 1, NPC_GID, true );
	view.setUint32( 6, capabilities, true );
	game.receive( { opcode: OP_SELECT_ACK, payload }, 1 );
	return { game, npc, sent };
}

/*
================
malformedAcks

Cover truncated special-trade replies, trailing bytes, and invalid status
bytes that must never be interpreted as ordinary refusals.
================
*/
function malformedAcks() {
	return [
		...Array.from( { length: SPECIAL_TRADE_ACK.length }, ( _, length ) => SPECIAL_TRADE_ACK.slice( 0, length ) ),
		Uint8Array.of( ...SPECIAL_TRADE_ACK, 0 ),
		Uint8Array.of( ...serviceAck( GACHA_MASK ), 0 ),
		Uint8Array.of( ...serviceAck( AVATAR_MASK ), 0 ),
		Uint8Array.of( 0, 4 ),
		Uint8Array.of( 3, 4 ),
		Uint8Array.of( 2 ),
		Uint8Array.of( 2, 4, 0 )
	];
}

test("inventory accepts special trade while both item windows are closed", () => {
	const owner = createInventory( () => {} );
	owner.bootstrap( { inventorySlotCount: 109, equipmentSlotCount: 13 } );
	owner.openShop( NPC_GID, 0, SPECIAL_TRADE_MASK );
	const before = owner.state();
	assert.equal( owner.receive( OP_NPC_ACK, SPECIAL_TRADE_ACK ), false );
	assert.deepEqual( owner.state(), before );
});

test("gameplay dispatch accepts the special-trade shop acknowledgement", () => {
	const { game, npc, sent } = selectedGame( SPECIAL_TRADE_MASK );
	try {
		game.command( { kind: "shop-open", gid: NPC_GID }, 2, npc );
		assert.equal( new DataView( sent.at( -1 ).payload.buffer ).getUint32( 4, true ), SPECIAL_TRADE_MASK );
		game.receive( { opcode: OP_NPC_ACK, payload: SPECIAL_TRADE_ACK }, 3 );
		const state = defined( game.take() );
		assert.equal( defined( state.gacha ).phase, "closed" );
		assert.equal( defined( state.magicOption ).phase, "closed" );
	} finally {
		game.dispose();
	}
});

for ( const window of WINDOWS ) {
	test(`${window.key}: inventory preserves a pending window across special trade`, () => {
		const owner = createInventory( () => {} );
		owner.process( { kind: window.command, gid: NPC_GID }, 0 );
		const before = owner.state();
		assert.equal( before[window.key].phase, "opening" );
		assert.equal( owner.receive( OP_NPC_ACK, SPECIAL_TRADE_ACK ), false );
		assert.deepEqual( owner.state(), before );
		assert.equal( owner.receive( OP_NPC_ACK, serviceAck( window.mask ) ), true );
		assert.equal( owner.state()[window.key].phase, "idle" );
		assert.equal( owner.state()[window.key].visible, true );
	});

	test(`${window.key}: gameplay preserves the pending window until its own acknowledgement`, () => {
		const { game, npc } = selectedGame( window.mask );
		try {
			game.command( { kind: window.command, gid: NPC_GID }, 2, npc );
			const before = defined( defined( game.take() )[window.key] );
			assert.equal( before.phase, "opening" );
			game.receive( { opcode: OP_NPC_ACK, payload: SPECIAL_TRADE_ACK }, 3 );
			assert.deepEqual( defined( game.take() )[window.key], before );
			game.receive( { opcode: OP_NPC_ACK, payload: serviceAck( window.mask ) }, 4 );
			const state = defined( defined( game.take() )[window.key] );
			assert.equal( state.phase, "idle" );
			assert.equal( state.visible, true );
		} finally {
			game.dispose();
		}
	});

	test(`${window.key}: malformed acknowledgements leave closed and pending sessions unchanged`, () => {
		for ( const opening of [ false, true ] ) {
			const owner = window.create();
			if ( opening ) owner.open( NPC_GID );
			const before = owner.state();
			for ( const payload of malformedAcks() ) {
				assert.throws( () => owner.opened( payload ), /Invalid NPC interaction/ );
				assert.deepEqual( owner.state(), before );
			}
			assert.equal( owner.opened( Uint8Array.of( 2, 4 ) ), opening );
			assert.equal( owner.state().phase, "closed" );
			assert.equal( owner.state().error, opening ? 4 : null );
		}
	});
}

test("inventory and gameplay reject malformed shared acknowledgements", () => {
	const inventory = createInventory( () => {} );
	const { game } = selectedGame( SPECIAL_TRADE_MASK );
	try {
		const before = inventory.state();
		for ( const payload of malformedAcks() ) {
			assert.throws( () => inventory.receive( OP_NPC_ACK, payload ), /Invalid NPC interaction/ );
			assert.deepEqual( inventory.state(), before );
			assert.throws( () => game.receive( { opcode: OP_NPC_ACK, payload }, 2 ), /Invalid NPC interaction/ );
		}
	} finally {
		game.dispose();
	}
});

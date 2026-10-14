/*
===========================================================================

gacha-window.ts - the Magic Pang window's rules, as pure functions

CIFGhaCha (resinfo ifghacha.txt, ginterface GDR_GHACHA) is a state machine
of one byte plus three repeating timers. Here the timers are functions of
elapsed time so the window keeps no clock of its own. The addresses cite the
v1.150 client; the research notes in ../research carry the full trail.

This module owns constants and arithmetic only. The state that evolves
(selected prize, registered card, the win flag) belongs to gacha-hud.ts.

===========================================================================
*/

// CIFNPCTalk row 0x27 appears when the NPC mask has this bit (5D9737).
export const GACHA_FUNCTION = 0x10000;

// CIFNPCTalk row 0x28 (5D9750): the exchange operator Gori, mask 0x20000.
export const GACHA_OPERATOR_FUNCTION = 0x20000;

// 642170 registers timer 2 at 0xFA0 ms; its handler sends 0x7053.
export const GACHA_ROLL_MS = 4000;
// Timers 1, 3 and 4 all fire every 0x64 ms.
export const GACHA_TICK_MS = 100;

// ifghacha.txt control ids.
export const GACHA_PRIZE_SLOT_FIRST = 100;
export const GACHA_PRIZE_SLOTS = 12;
export const GACHA_CARD_SLOT = 112;
export const GACHA_PLAY_BUTTON = 10;
export const GACHA_REEL_LEFT = 20;
export const GACHA_REEL_RIGHT = 21;
export const GACHA_EFFECT = 50;
export const GACHA_DECO = 51;
export const GACHA_TEXT = 36;

// Reel frames gacha_roll0000..0020, even numbers only (643070 id 1).
const REEL_LAST_FRAME = 20;
const REEL_STEP = 2;

// Effect frames ga_effect_01..19. The frame holds on these tick ranges (643070 id 3).
const EFFECT_HOLD_A_FIRST = 7;
const EFFECT_HOLD_A_LAST = 15;
const EFFECT_HOLD_B_FIRST = 23;
const EFFECT_HOLD_B_LAST = 35;
// The run ends when the tick counter exceeds this (643070: cmp [0xCEA7E8], 0x28).
export const GACHA_EFFECT_LAST_TICK = 40;

// DECO sprite sheets: alcm_effect_prepare is 64x64 x 16; ga_effect_sucess is 64x81 x 15.
export type GachaSheetKind = "prepare" | "success";
export interface GachaSheet {
	readonly frameWidth: number;
	readonly frameHeight: number;
	readonly frames: number;
}

/*
================
gachaSheet

Frame size and count the OnCreate (644C20) and the effect end (643070) give the
DECO static: 64x64 x 16 for the prepare sheet, 64x81 x 15 for the success sheet.
================
*/
export function gachaSheet( kind: GachaSheetKind ): GachaSheet {
	return kind === "success" ?
		{ frameWidth: 64, frameHeight: 81, frames: 15 } :
		{ frameWidth: 64, frameHeight: 64, frames: 16 };
}

export type GachaUiState = 0 | 1 | 2;

/*
================
gachaUiState

6423C0: 0 until a prize is chosen, 1 until a gacha card sits in the card slot,
then 2, the only state in which Play is enabled.
================
*/
export function gachaUiState( prizeSelected: boolean, cardRegistered: boolean ): GachaUiState {
	if ( !prizeSelected ) return 0;
	return cardRegistered ? 2 : 1;
}

/*
================
gachaStateText

The instruction line (control 36) for each state.
================
*/
export function gachaStateText( state: GachaUiState ): string {
	switch ( state ) {
		case 0:
			return "UIIT_MSG_GACHA_CHOICE";
		case 1:
			return "UIIT_MSG_GACHA_ENTRY";
		default:
			return "UIIT_MSG_GACHA_BUTTON_CLICK";
	}
}

/*
================
gachaReelStep

Timer 1 (643070): the counter wraps to 0 when it is past 20, names
gacha_roll%04d from it, then advances by 2. Returns the frame to draw and the
counter for the next tick. The counter survives between rolls.
================
*/
export function gachaReelStep( counter: number ): { frame: number; next: number; } {
	const frame = counter > REEL_LAST_FRAME ? 0 : counter;
	return { frame, next: frame + REEL_STEP };
}

/*
================
gachaEffectHolds

True when the tick is inside a range where the effect frame does not advance.
================
*/
export function gachaEffectHolds( tick: number ): boolean {
	return tick >= EFFECT_HOLD_A_FIRST && tick <= EFFECT_HOLD_A_LAST ||
		tick >= EFFECT_HOLD_B_FIRST && tick <= EFFECT_HOLD_B_LAST;
}

/*
================
gachaEffectFrame

Timer 3 (643070): ga_effect_%02d names the counter, which starts at 1 and
advances on every tick that does not hold. Tick 0 draws frame 1; tick 40, the
last, draws frame 19.
================
*/
export function gachaEffectFrame( tick: number ): number {
	let frame = 1;
	for ( let t = 0; t < tick; t++ ) if ( !gachaEffectHolds( t ) ) frame++;
	return frame;
}

/*
================
gachaSheetFrameRect

Where a sprite-sheet frame sits in its texture, as normalized UVs: row-major
cells of frame size across the texture width (51F140, branch that divides by
the frame size fields at +0x38C/+0x390).
================
*/
export function gachaSheetFrameRect(
	sheet: GachaSheet,
	frame: number,
	texture: readonly [number, number]
): [number, number, number, number] {
	const columns = Math.max( 1, Math.floor( texture[0] / sheet.frameWidth ) );
	const column = frame % columns, row = Math.floor( frame / columns );
	return [
		column * sheet.frameWidth / texture[0],
		row * sheet.frameHeight / texture[1],
		sheet.frameWidth / texture[0],
		sheet.frameHeight / texture[1]
	];
}

/*
================
isGachaVoucher

642270: the bag test behind the operator row. A reference word is a result voucher when
its packed type ids read 3.3.14.2: bit 1 clear, then (w & 0x1C) == 0xC, (w & 0x60) == 0x60,
(w & 0x780) == 0x700 and (w & 0xF800) == 0x1000. Bit 0 is not tested.
================
*/
export function isGachaVoucher( typeFlags: number ): boolean {
	return (typeFlags & 0x2) === 0 && (typeFlags & 0x1c) === 0xc && (typeFlags & 0x60) === 0x60 &&
		(typeFlags & 0x780) === 0x700 && (typeFlags & 0xf800) === 0x1000;
}

/*
================
gachaPrizeEntry

A prize slot control id (100..111) selects the lottery entry CommandID + 1.
INFERENCE: 644410 fills the slots from the NPC's lottery set, whose selection
ids are 1..12 (gacha-catalog.ts), in file order; the slot-to-entry order has
not been read from 644410 itself.
================
*/
export function gachaPrizeEntry( controlId: number ): number | null {
	const index = controlId - GACHA_PRIZE_SLOT_FIRST;
	return index >= 0 && index < GACHA_PRIZE_SLOTS ? index + 1 : null;
}

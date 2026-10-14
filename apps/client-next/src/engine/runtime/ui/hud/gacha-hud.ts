/*
===========================================================================

gacha-hud.ts - the Magic Pang window's state: selection, card, timers

Owns what CIFGhaCha keeps in the object between frames: the chosen prize, the
card registered in the slot, the reel and effect counters, the DECO sheet and
the win flag. The wire and the 4000 ms roll belong to the worker's gacha.ts;
this module only presents them. The rules are in foundation/ui/gacha-window.ts.

The three native timers (643070) are replayed from the simulation clock: each
has a next-fire time and fires in order up to `now`, so a stalled frame
catches up exactly as the repeating timers would have.

===========================================================================
*/
import {
	GACHA_ROLL_MS,
	gachaSheet,
	GACHA_TICK_MS,
	gachaEffectHolds,
	gachaReelStep,
	gachaStateText,
	gachaUiState,
	GACHA_EFFECT_LAST_TICK
} from "@/engine/foundation/ui/gacha-window";
import type { GachaSheetKind } from "@/engine/foundation/ui/gacha-window";
import { isGachaTicket } from "@/engine/foundation/gameplay/gacha-catalog";
import type { GachaState } from "@/engine/contracts/item-process";
import type { InventoryItem } from "@/engine/contracts/gameplay";

// 643070 id 4 stops the success sheet when its frame reaches 14.
const SUCCESS_LAST_FRAME = 14;
// A reopened window must not replay minutes of ticks in one frame.
const MAX_TIMER_FIRES = 256;
// Bag slots start at 13 (642170 adds 0xD to the card slot index).
const FIRST_BAG_SLOT = 13;

export interface GachaFrameInput {
	readonly gacha: GachaState | undefined;
	readonly now: number;
	readonly items: readonly InventoryItem[];
}

export interface GachaFrame {
	readonly state: 0 | 1 | 2;
	// A symbol for the instruction line, or null while a result line is shown.
	readonly text: string | null;
	readonly result: "win" | "lose" | null;
	readonly reward: { readonly refObjId: number; readonly quantity: number; } | null;
	readonly entry: number;
	readonly cardSlot: number;
	readonly busy: boolean;
	readonly playEnabled: boolean;
	// gacha_roll%04d number while the reels spin; null shows gacha_roll_original.
	readonly reelFrame: number | null;
	// ga_effect_%02d number on control 50.
	readonly effectFrame: number;
	readonly deco: {
		readonly sheet: GachaSheetKind;
		readonly frame: number;
	} | null;
}

/*
================
createGachaHud
================
*/
export function createGachaHud() {
	let entry = 0, cardSlot = -1, override: { result: "win" | "lose"; reward: GachaFrame["reward"]; } | null = null;
	// Counters that survive between rolls in the native object (+0x8DC, +0x8E0, 0xCEA7E8, +0x8E4).
	let reelCounter = 0, effectFrame = 1, effectShown = 1, tick = 0, winFlag = false;
	let reelFrame: number | null = null, decoVisible = false, success = false, decoFrame = 0;
	let reelNext: number | null = null, effectNext: number | null = null, successNext: number | null = null;
	let rollStarted = -1, resultApplied = -1, spinning = false, bagPage = 0;
	// The operator whose "no voucher" line the talk window shows (5DB746), or 0.
	let noVoucherNpc = 0;

	/*
	================
	fireReel

	Timer 1: name the frame, then step by two.
	================
	*/
	function fireReel() {
		const step = gachaReelStep( reelCounter );
		reelFrame = step.frame;
		reelCounter = step.next;
		reelNext! += GACHA_TICK_MS;
	}

	/*
	================
	fireEffect

	Timer 3: show the current frame; frames advance unless the tick is in a hold
	range; past the last tick the run ends, and a win flag set by then starts the
	success sheet. A loss leaves the tick counter at 1 (643070 6434E1), so the next
	run starts one tick in. That quirk is native.
	================
	*/
	function fireEffect() {
		const at = effectNext!;
		if ( tick <= GACHA_EFFECT_LAST_TICK ) {
			effectShown = effectFrame;
			if ( !gachaEffectHolds( tick ) ) effectFrame++;
			tick++;
			decoFrame = (decoFrame + 1) % gachaSheet( success ? "success" : "prepare" ).frames;
			effectNext = at + GACHA_TICK_MS;
			return;
		}
		effectNext = null;
		effectFrame = 1;
		effectShown = 1;
		decoVisible = false;
		if ( winFlag ) {
			tick = 0;
			success = true;
			decoFrame = 1;
			decoVisible = true;
			successNext = at + GACHA_TICK_MS;
		} else {
			tick = 1;
			decoFrame = (decoFrame + 1) % gachaSheet( "prepare" ).frames;
		}
	}

	/*
	================
	fireSuccess

	Timer 4: at frame 14 the sheet stops and the prepare sheet returns.
	================
	*/
	function fireSuccess() {
		if ( decoFrame === SUCCESS_LAST_FRAME ) {
			successNext = null;
			decoVisible = false;
			success = false;
			decoFrame = 0;
			return;
		}
		decoFrame = (decoFrame + 1) % gachaSheet( "success" ).frames;
		successNext! += GACHA_TICK_MS;
	}

	/*
	================
	advance

	Starts a run when the worker reports a new roll, stops the reels at 4000 ms
	(timer 2 kills timer 1 as it sends), then fires due timers in time order.
	================
	*/
	function advance( gacha: GachaState | undefined, now: number ) {
		if ( gacha?.phase === "rolling" && gacha.started !== rollStarted ) {
			rollStarted = gacha.started;
			decoVisible = true;
			spinning = true;
			reelNext = gacha.started + GACHA_TICK_MS;
			effectNext = gacha.started + GACHA_TICK_MS;
		}
		const endOfSpin = rollStarted + GACHA_ROLL_MS;
		for ( let fires = 0; fires < MAX_TIMER_FIRES; fires++ ) {
			const due = [ reelNext, effectNext, successNext ].filter( t => t !== null && t <= now ) as number[];
			if ( due.length === 0 ) break;
			const first = Math.min( ...due );
			if ( spinning && first >= endOfSpin && first === reelNext ) {
				spinning = false;
				reelNext = null;
				reelFrame = null;
				continue;
			}
			if ( first === reelNext ) fireReel();
			else if ( first === effectNext ) fireEffect();
			else fireSuccess();
		}
		if ( spinning && now >= endOfSpin ) {
			spinning = false;
			reelNext = null;
			reelFrame = null;
		}
	}

	/*
	================
	applyResult

	643DF0 sets the win flag when the result arrives and replaces the instruction
	line with the win or lose message. Applied once per roll.
	================
	*/
	function applyResult( gacha: GachaState ) {
		if ( gacha.phase !== "result" || gacha.result === null || gacha.started === resultApplied ) return;
		resultApplied = gacha.started;
		winFlag = gacha.result === "win";
		override = { result: gacha.result, reward: gacha.reward ?? null };
	}

	/*
	================
	frame
	================
	*/
	function frame( input: GachaFrameInput ): GachaFrame {
		const { gacha, now } = input;
		if ( gacha ) applyResult( gacha );
		advance( gacha, now );
		const busy = gacha?.phase === "rolling" || gacha?.phase === "waiting";
		const card = cardSlot >= 0 ? input.items.find( row => row.slot === cardSlot ) : undefined;
		const registered = !!card && card.quantity > 0 && isGachaTicket( card.typeFlags );
		const state = gachaUiState( entry > 0, registered );
		return {
			state,
			text: override ? null : gachaStateText( state ),
			result: override?.result ?? null,
			reward: override?.reward ?? null,
			entry,
			cardSlot,
			busy,
			playEnabled: !busy && state === 2,
			reelFrame,
			effectFrame: effectShown,
			deco: decoVisible ?
				{ sheet: success ? "success" : "prepare", frame: decoFrame } :
				null
		};
	}

	return {
		/*
		================
		reset

		The window opens with nothing chosen. Counters persist: the native object
		keeps them. INFERENCE: native keeps the chosen prize and card across opens;
		the port clears them because the bag may have changed meanwhile.
		================
		*/
		reset() {
			entry = 0;
			cardSlot = -1;
			override = null;
		},
		/*
		================
		select

		642550: a click on a prize slot while not busy.
		================
		*/
		select( next: number, busy: boolean ) {
			if ( busy || next < 1 ) return false;
			entry = next;
			override = null;
			return true;
		},
		/*
		================
		drop

		645180: a bag item dropped on the card slot is accepted only while a prize
		is chosen and no card is registered, and only if it is a gacha card.
		================
		*/
		drop( slot: number, item: InventoryItem | undefined, busy: boolean, registered: boolean ) {
			if ( busy || entry === 0 || registered ) return false;
			if (
				!item || item.slot !== slot || slot < FIRST_BAG_SLOT || item.quantity <= 0 ||
				!isGachaTicket( item.typeFlags )
			) {
				return false;
			}
			cardSlot = slot;
			override = null;
			return true;
		},
		/*
		================
		clearCard

		645180: the card slot's own item dropped elsewhere empties the slot.
		================
		*/
		clearCard( busy: boolean ) {
			if ( busy || cardSlot < 0 ) return false;
			cardSlot = -1;
			override = null;
			return true;
		},
		/*
		================
		play

		642170: the roll to send, or null while Play is refused.
		================
		*/
		play( input: GachaFrameInput ): { entry: number; slot: number; } | null {
			const current = frame( input );
			return current.playEnabled ? { entry, slot: cardSlot } : null;
		},
		frame,
		/*
		================
		noVoucher

		5DB739..5DB763: the operator row with no voucher in the bag answers with
		SN_TALK_CH_GACHA_OPERATOR_3 in the talk window instead of sending a request.
		================
		*/
		noVoucher( gid: number ) {
			noVoucherNpc = gid;
		},
		noVoucherFor() {
			return noVoucherNpc;
		},
		clearNoVoucher() {
			noVoucherNpc = 0;
		},
		/*
		================
		bag

		The embedded bag's page; the window clamps it to the pages the bag has.
		================
		*/
		bag( pages: number ) {
			bagPage = Math.max( 0, Math.min( pages - 1, bagPage ) );
			return bagPage;
		},
		setBag( page: number ) {
			bagPage = Math.max( 0, page );
		}
	};
}

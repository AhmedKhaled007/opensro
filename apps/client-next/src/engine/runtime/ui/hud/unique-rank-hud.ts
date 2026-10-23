/*
===========================================================================

unique-rank-hud.ts - the unique damage ranking window (port-only, not native)

Opens once for every ranking the worker publishes (0x3FB7, sent only with
SRO_UNIQUE_DAMAGE_RANK on) and stays open until the player closes it. A
newer kill replaces the open ranking. The original has no such window.

===========================================================================
*/
import type { GameplayState, UniqueDamageRank } from "@/engine/contracts/gameplay";

/*
================
createUniqueRankHud
================
*/
export function createUniqueRankHud() {
	let shown: UniqueDamageRank | null = null;
	let seen = 0;
	return {
		/*
		================
		state
		================
		*/
		state: () => shown,
		/*
		================
		reconcile

		A ranking newer than the last one opens the window; one already seen
		never reopens it after the player closed it. True when it opened.
		================
		*/
		reconcile( gameplay: Pick<GameplayState, "uniqueDamageRank"> | null | undefined ): boolean {
			const next = gameplay?.uniqueDamageRank;
			if ( !next || next.sequence <= seen ) return false;
			shown = next;
			seen = next.sequence;
			return true;
		},
		/*
		================
		close
		================
		*/
		close() {
			shown = null;
		},
		/*
		================
		reset

		A new session's worker numbers its rankings from 1 again.
		================
		*/
		reset( sessionEnded = false ) {
			shown = null;
			if ( sessionEnded ) seen = 0;
		}
	};
}

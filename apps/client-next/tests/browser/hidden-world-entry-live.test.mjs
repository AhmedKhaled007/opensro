/*
===========================================================================

hidden-world-entry-live.test.mjs - world entry completes in a background tab

A world entry or teleport is finished only when the client sends 0x3012
game-ready, and the server keeps the scene inert until then. Scene admission
runs inside the renderer frame, so a tab hidden during the load must still
admit the scene. The native client keeps updating while inactive and finishes
its loading screen in CPSMission_OnUpdate (728E10); only drawing stops
(A11650). The tab is hidden by a real tab switch in a headed browser, never
by spoofing visibilityState, because only a real hidden document stops
animation frames.

===========================================================================
*/
import assert from "node:assert/strict";
import { test } from "node:test";
import { resolveProbeCharacter } from "../../../../scripts/lib/probeCharacter.mjs";
import { launchProbeBrowser } from "../../../../scripts/lib/probeBrowser.mjs";
import { enterPlayableSession, waitPlayableWorld } from "./helpers/playable-session.mjs";

// Windows window occlusion would hide every tab; only the tab switch may.
const TAB_VISIBILITY_ONLY = "--disable-features=CalculateNativeWinOcclusion";

/*
================
hidden world entry
================
*/
test( "a world entry started in a hidden tab reaches game-ready without the tab returning", {
	timeout: 180000
}, async () => {
	const { browser, page } = await launchProbeBrowser( {
		headed: true,
		backgroundThrottling: true,
		extraBrowserArgs: [ TAB_VISIBILITY_ONLY ]
	} );
	try {
		const character = await enterPlayableSession( page, resolveProbeCharacter() );
		const other = await page.context().newPage();
		await other.goto( "about:blank" );
		await other.bringToFront();
		await page.waitForFunction( () => document.visibilityState === "hidden", null, { polling: 100 } );
		await waitPlayableWorld( page, character );
		// The load must have finished while hidden, not after a late refocus.
		assert.equal( await page.evaluate( () => document.visibilityState ), "hidden" );
		const report = await page.evaluate( () => document.querySelector( "output" )?.textContent ?? "" );
		assert.match( report, /World admission: \S+; 0 pending groups/ );
	} finally {
		await browser.close();
	}
} );

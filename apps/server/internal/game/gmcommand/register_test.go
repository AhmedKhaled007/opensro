/*
===========================================================================

register_test.go - command diagnostics preserve the privilege audit boundary

===========================================================================
*/
package gmcommand

import (
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
)

/*
================
TestGMRequestDiagnosticsSkipDeniedPayloads
================
*/
func TestGMRequestDiagnosticsSkipDeniedPayloads(t *testing.T) {
	logger := log.StandardLogger()
	oldHooks := logger.ReplaceHooks(make(log.LevelHooks))
	t.Cleanup(func() { logger.ReplaceHooks(oldHooks) })
	hook := logtest.NewGlobal()
	for range 100 {
		logGMRequest(OpGmCommand, "Player", []byte{SubMobKill}, Outcome{PrivilegeDenied: true})
	}
	if len(hook.AllEntries()) != 0 {
		t.Fatal("denied requests bypassed the rate-limited audit")
	}
	logGMRequest(OpGmCommand, "GM", []byte{SubMobKill}, Outcome{Ack: []byte{AckResultOK, SubMobKill}})
	if len(hook.AllEntries()) != 1 || !strings.Contains(hook.LastEntry().Message, "payload=14") {
		t.Fatal("privileged command lost its discriminator diagnostic")
	}
	hook.Reset()
	logGMRequest(OpGmCommand, "GM", make([]byte, gmPayloadLogBytes*100), Outcome{})
	if len(hook.LastEntry().Message) > gmPayloadLogBytes*4+200 {
		t.Fatal("unbounded payload diagnostic")
	}
}

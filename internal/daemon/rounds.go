package daemon

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/hocoder-agents/crush-bot/internal/group"
)

// Round requests are claimed by the daemon: rename is the claim (a
// second sweeper can never see the same request). The settle loop then
// runs in its own goroutine so the poll loop keeps waking DMs.
func sweepRounds(ctx context.Context, o Options, inflight *sync.Map) {
	groups, err := group.List(o.Root)
	if err != nil {
		return
	}
	for _, g := range groups {
		group.FlushStaleSays(o.Root, g.ID)
		reqPath := group.RequestPath(o.Root, g.ID)
		if _, err := os.Stat(reqPath); err != nil {
			continue
		}
		if _, busy := inflight.Load(g.ID); busy {
			continue
		}
		claim := reqPath + ".claimed"
		if err := os.Rename(reqPath, claim); err != nil {
			continue
		}
		b, err := os.ReadFile(claim)
		if err != nil {
			_ = os.Remove(claim)
			continue
		}
		_ = os.Remove(claim)
		var req group.RoundRequest
		if json.Unmarshal(b, &req) != nil || req.Line == "" {
			o.log(g.ID, "round_skip", "empty request", "", 0)
			continue
		}
		inflight.Store(g.ID, true)
		go func(gid string, g group.Group, line string) {
			defer inflight.Delete(gid)
			start := time.Now()
			o.log(gid, "round_start", "", "", 0)
			runCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
			defer cancel()
			err := group.RunUntilSettle(runCtx, o.Cfg, o.Bin, o.Root, g, line)
			reason := ""
			if err != nil {
				reason = err.Error()
			}
			o.log(gid, "round_done", reason, "", time.Since(start).Milliseconds())
			group.FlushStaleSays(o.Root, g.ID)
		}(g.ID, g, req.Line)
	}
}

package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hocoder-agents/crush-bot/internal/config"
	"github.com/hocoder-agents/crush-bot/internal/crush"
	"github.com/hocoder-agents/crush-bot/internal/envelope"
	"github.com/hocoder-agents/crush-bot/internal/lock"
	"github.com/hocoder-agents/crush-bot/internal/roster"
)

type Options struct {
	Root    string
	Bin     string
	Cfg     config.Config
	Poll    time.Duration
	LogPath string
}

type logLine struct {
	TS       string `json:"ts"`
	Level    string `json:"level"`
	Bot      string `json:"bot,omitempty"`
	Event    string `json:"event"`
	Reason   string `json:"reason,omitempty"`
	Envelope string `json:"envelope_id,omitempty"`
	MS       int64  `json:"duration_ms,omitempty"`
}

func (o Options) log(bot, event, reason, eid string, ms int64) {
	path := o.LogPath
	if path == "" {
		path = logPath(o.Root)
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	rec := logLine{
		TS: time.Now().UTC().Format(time.RFC3339Nano), Level: "info",
		Bot: bot, Event: event, Reason: reason, Envelope: eid, MS: ms,
	}
	b, _ := json.Marshal(rec)
	_, _ = f.Write(append(b, '\n'))
}

func Run(ctx context.Context, o Options) error {
	if o.Poll <= 0 {
		o.Poll = 200 * time.Millisecond
	}
	if err := os.MkdirAll(filepath.Join(o.Root, "logs"), 0o700); err != nil {
		return err
	}
	if err := WritePID(o.Root, os.Getpid()); err != nil {
		return err
	}
	defer os.Remove(pidPath(o.Root))
	t := time.NewTicker(o.Poll)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			_, _ = Once(ctx, o)
		}
	}
}

func Once(ctx context.Context, o Options) (int, error) {
	bots, err := roster.List(o.Root, true)
	if err != nil {
		return 0, err
	}
	sort.Slice(bots, func(i, j int) bool { return bots[i].Slug < bots[j].Slug })
	max := o.Cfg.MaxParallel
	if max <= 0 {
		max = 4
	}
	if max > 8 {
		max = 8
	}
	sem := make(chan struct{}, max)
	var wg sync.WaitGroup
	var mu sync.Mutex
	woke := 0
	for _, bot := range bots {
		home := roster.Home(o.Root, bot.Slug)
		expireOld(home, o.Cfg.QueuedExpire)
		_, paths, err := envelope.List(envelope.PendingDir(home))
		if err != nil || len(paths) == 0 {
			continue
		}
		sem <- struct{}{}
		wg.Add(1)
		bot := bot
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			n, err := wakeBot(ctx, o, bot)
			if err != nil {
				o.log(bot.Slug, "wake_error", err.Error(), "", 0)
			}
			mu.Lock()
			woke += n
			mu.Unlock()
		}()
	}
	wg.Wait()
	sweepRounds(ctx, o, &rounds)
	return woke, nil
}

// rounds tracks in-flight settle loops per room so the sweeper never
// double-runs a room and recovery leaves live rounds alone.
var rounds sync.Map

func expireOld(home string, maxAge time.Duration) {
	if maxAge <= 0 {
		maxAge = 24 * time.Hour
	}
	_, paths, err := envelope.List(envelope.PendingDir(home))
	if err != nil {
		return
	}
	cut := time.Now().Add(-maxAge)
	for _, p := range paths {
		env, err := envelope.ReadFile(p)
		if err != nil {
			continue
		}
		if !env.CreatedAt.IsZero() && env.CreatedAt.Before(cut) {
			_, _ = envelope.Write(envelope.FailedDir(home), env)
			_ = os.Remove(p)
		}
	}
}

func wakeBot(ctx context.Context, o Options, bot roster.Bot) (int, error) {
	home := roster.Home(o.Root, bot.Slug)
	_, _ = crush.ReclaimStale(home)
	l, err := lock.Acquire(crush.LockPath(home), 0, true)
	if err != nil {
		return 0, nil // busy: skip
	}
	defer l.Unlock()

	batch, srcs, err := coalesce(home, o.Cfg.CoalesceInbox)
	if err != nil || len(batch) == 0 {
		return 0, err
	}
	var refs []crush.InboundRef
	maxHop := 0
	var trace []string
	seen := map[string]bool{}
	parent := ""
	for i, e := range batch {
		if i == 0 {
			parent = e.ID
		}
		if e.Hop > maxHop {
			maxHop = e.Hop
		}
		for _, t := range e.Trace {
			if !seen[t] {
				seen[t] = true
				trace = append(trace, t)
			}
		}
		refs = append(refs, crush.InboundRef{
			ID: e.ID, Kind: e.Kind, From: e.From, Hop: e.Hop, Trace: e.Trace, TaskID: e.TaskID,
		})
		if _, err := envelope.Move(srcs[i], envelope.ProcessingDir(home)); err != nil {
			return 0, err
		}
	}

	start := time.Now()
	_, runErr := crush.Run(ctx, crush.RunOpts{
		Bot:        bot,
		Root:       o.Root,
		Bin:        o.Bin,
		Kind:       "wake",
		Prompt:     WakePrompt(batch),
		Timeout:    o.Cfg.TurnLockTimeout,
		Debug:      os.Getenv("CRUSHBOT_DEBUG") == "1",
		Yolo:       bot.Unattended == "yolo",
		Inbound:    refs,
		InboundHop: maxHop,
		Trace:      trace,
		ParentID:   parent,
		MaxHops:    o.Cfg.MaxHops,
		HeldLock:   l,
	})
	ms := time.Since(start).Milliseconds()

	exit := 0
	stderr, stdout := "", ""
	if runErr != nil {
		exit = 1
		stderr = runErr.Error()
	}
	reason, retryable := Classify(exit, stderr, stdout)
	if runErr == nil {
		for i := range batch {
			env := batch[i]
			_, _ = envelope.Move(filepath.Join(envelope.ProcessingDir(home), env.ID+".json"), envelope.ArchiveDir(home))
			o.log(bot.Slug, "archived", "", env.ID, ms)
		}
		writeReceipts(o, bot, batch, maxHop, trace)
		return 1, nil
	}

	for i := range batch {
		p := filepath.Join(envelope.ProcessingDir(home), batch[i].ID+".json")
		env := batch[i]
		if retryable && env.Attempt < 1 {
			env.Attempt++
			_ = os.Remove(p)
			_, _ = envelope.WritePending(home, env)
			o.log(bot.Slug, "retry", reason, env.ID, ms)
			continue
		}
		_ = os.Remove(p)
		_, _ = envelope.Write(envelope.FailedDir(home), env)
		o.log(bot.Slug, "failed", reason, env.ID, ms)
	}
	return 1, runErr
}

func coalesce(home string, capn int) ([]envelope.Envelope, []string, error) {
	if capn <= 0 {
		capn = 8
	}
	envs, paths, err := envelope.List(envelope.PendingDir(home))
	if err != nil {
		return nil, nil, err
	}
	type pair struct {
		e envelope.Envelope
		p string
	}
	var ps []pair
	for i := range envs {
		ps = append(ps, pair{envs[i], paths[i]})
	}
	sort.Slice(ps, func(i, j int) bool { return ps[i].e.ID < ps[j].e.ID })
	var out []envelope.Envelope
	var srcs []string
	size := 0
	for _, x := range ps {
		n := len(x.e.Body)
		if len(out) >= capn || (size+n > 32*1024 && len(out) > 0) {
			break
		}
		out = append(out, x.e)
		srcs = append(srcs, x.p)
		size += n
	}
	return out, srcs, nil
}

func writeReceipts(o Options, bot roster.Bot, batch []envelope.Envelope, inboundHop int, trace []string) {
	seen := map[string]bool{}
	hadDM := false
	for _, e := range batch {
		if e.Kind == "dm" {
			hadDM = true
		}
	}
	if !hadDM {
		return
	}
	text, err := crush.LastAssistant(o.Bin, roster.Home(o.Root, bot.Slug), filepath.Join(roster.Home(o.Root, bot.Slug), ".crush"), bot.CanonicalSessionID, 4096)
	if err != nil || strings.TrimSpace(text) == "" {
		text = "(no assistant text)"
	}
	hop := inboundHop + 1
	max := o.Cfg.MaxHops
	if max <= 0 {
		max = 8
	}
	trc := append([]string{}, trace...)
	trc = append(trc, bot.Slug)
	for _, e := range batch {
		if e.Kind != "dm" || e.From == "" || e.From == "user" || seen[e.From] {
			continue
		}
		seen[e.From] = true
		if hop > max {
			needsYou(o.Root, bot.Slug, "hop_limit", "receipt dropped for @"+e.From)
			continue
		}
		if !roster.Exists(o.Root, e.From) {
			continue
		}
		rec := envelope.Envelope{
			Kind:        "receipt",
			From:        bot.Slug,
			To:          e.From,
			Hop:         hop,
			Trace:       trc,
			Body:        text,
			Attribution: fmt.Sprintf("FYI receipt from %s (@%s):", bot.Slug, bot.Slug),
		}
		pid := e.ID
		rec.ParentID = &pid
		_, _ = envelope.WritePending(roster.Home(o.Root, e.From), rec)
		o.log(bot.Slug, "receipt", "", e.ID, 0)
	}
}

func needsYou(root, bot, reason, summary string) {
	path := filepath.Join(root, "needs_you.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	rec := map[string]any{"ts": time.Now().UTC().Format(time.RFC3339), "bot": bot, "reason": reason, "summary": summary}
	b, _ := json.Marshal(rec)
	_, _ = f.Write(append(b, '\n'))
}

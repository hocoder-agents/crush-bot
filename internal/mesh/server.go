package mesh

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/hocoder-agents/crush-bot/internal/version"
)

type rpcReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcRes struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcErr         `json:"error,omitempty"`
}

type rpcErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func Serve(in io.Reader, out io.Writer, id Identity) error {
	dec := json.NewDecoder(in)
	enc := json.NewEncoder(out)
	for {
		var req rpcReq
		if err := dec.Decode(&req); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if req.Method == "notifications/initialized" || req.Method == "initialized" {
			continue
		}
		res := handle(req, id)
		if req.ID == nil {
			continue
		}
		if err := enc.Encode(res); err != nil {
			return err
		}
	}
}

func handle(req rpcReq, id Identity) rpcRes {
	ok := func(v any) rpcRes {
		return rpcRes{JSONRPC: "2.0", ID: req.ID, Result: v}
	}
	fail := func(msg string) rpcRes {
		return rpcRes{JSONRPC: "2.0", ID: req.ID, Error: &rpcErr{Code: -32000, Message: msg}}
	}
	switch req.Method {
	case "initialize":
		return ok(map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "crushbot-mesh", "version": version.Version},
		})
	case "ping":
		return ok(map[string]any{})
	case "tools/list":
		return ok(map[string]any{"tools": toolDefs()})
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return fail(err.Error())
		}
		body, isErr := callTool(id, p.Name, p.Arguments)
		return ok(map[string]any{
			"content": []map[string]string{{"type": "text", "text": body}},
			"isError": isErr,
		})
	default:
		return fail("unknown method " + req.Method)
	}
}

func toolDefs() []map[string]any {
	return []map[string]any{
		{
			"name":        "message_bot",
			"description": "Fire-and-forget DM to another bot. Compose your own message. Returns queued or sent. Do not retry queued/sent. Do not wait for a reply.",
			"inputSchema": map[string]any{
				"type":     "object",
				"required": []string{"target", "message"},
				"properties": map[string]any{
					"target":  map[string]any{"type": "string", "description": "Bot slug, without @"},
					"message": map[string]any{"type": "string", "maxLength": MessageMaxChars},
				},
			},
		},
		{
			"name":        "roster_list",
			"description": "List crushbot roster slugs, titles, and roles.",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			"name":        "escalate_to_human",
			"description": "Flag the operator. Use for judgment calls.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"summary": map[string]any{"type": "string"},
				},
			},
		},
		{
			"name":        "assign_task",
			"description": "Queue a durable task for another bot. They are woken asynchronously.",
			"inputSchema": map[string]any{
				"type":     "object",
				"required": []string{"target", "title", "body"},
				"properties": map[string]any{
					"target":          map[string]any{"type": "string"},
					"title":           map[string]any{"type": "string", "maxLength": 200},
					"body":            map[string]any{"type": "string", "maxLength": MessageMaxChars},
					"priority":        map[string]any{"type": "string", "enum": []string{"low", "normal", "high"}},
					"idempotency_key": map[string]any{"type": "string", "maxLength": 200},
				},
			},
		},
		{
			"name":        "task_list",
			"description": "List tasks where you are assignee or assigner.",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			"name":        "task_complete",
			"description": "Mark a task done before ending the turn.",
			"inputSchema": map[string]any{
				"type":     "object",
				"required": []string{"id"},
				"properties": map[string]any{
					"id":     map[string]any{"type": "string"},
					"result": map[string]any{"type": "string"},
				},
			},
		},
		{
			"name":        "task_fail",
			"description": "Mark a task failed or blocked (need_human).",
			"inputSchema": map[string]any{
				"type":     "object",
				"required": []string{"id"},
				"properties": map[string]any{
					"id":     map[string]any{"type": "string"},
					"reason": map[string]any{"type": "string"},
					"error":  map[string]any{"type": "string"},
				},
			},
		},
		{
			"name":        "group_say",
			"description": "Public line in the current group room. Not a private DM.",
			"inputSchema": map[string]any{
				"type":     "object",
				"required": []string{"text"},
				"properties": map[string]any{
					"text": map[string]any{"type": "string"},
				},
			},
		},
		{
			"name":        "group_pass",
			"description": "Skip this group round (PASS).",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			"name":        "task_delegate",
			"description": "Create a child task and wait. Parent becomes waiting_child.",
			"inputSchema": map[string]any{
				"type":     "object",
				"required": []string{"id", "target", "title", "body"},
				"properties": map[string]any{
					"id":     map[string]any{"type": "string"},
					"target": map[string]any{"type": "string"},
					"title":  map[string]any{"type": "string"},
					"body":   map[string]any{"type": "string"},
				},
			},
		},
	}
}

func callTool(id Identity, name string, args json.RawMessage) (string, bool) {
	switch name {
	case "message_bot":
		var a struct {
			Target  string `json:"target"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(args, &a)
		r := MessageBot(id, a.Target, a.Message)
		return marshal(r), r.Reason != ""
	case "roster_list":
		list, r := RosterList(id)
		if r.Reason != "" {
			return marshal(r), true
		}
		return marshal(list), false
	case "escalate_to_human":
		var a struct {
			Summary string `json:"summary"`
		}
		_ = json.Unmarshal(args, &a)
		r := Escalate(id, a.Summary)
		return marshal(r), r.Reason != ""
	case "assign_task":
		var a struct {
			Target         string `json:"target"`
			Title          string `json:"title"`
			Body           string `json:"body"`
			Priority       string `json:"priority"`
			IdempotencyKey string `json:"idempotency_key"`
		}
		_ = json.Unmarshal(args, &a)
		r := AssignTask(id, a.Target, a.Title, a.Body, a.Priority, a.IdempotencyKey)
		return marshal(r), r.Reason != ""
	case "task_list":
		list, r := TaskList(id)
		if r.Reason != "" {
			return marshal(r), true
		}
		return marshal(list), false
	case "task_complete":
		var a struct {
			ID     string `json:"id"`
			Result string `json:"result"`
		}
		_ = json.Unmarshal(args, &a)
		r := TaskComplete(id, a.ID, a.Result)
		return marshal(r), r.Reason != ""
	case "task_fail":
		var a struct {
			ID     string `json:"id"`
			Reason string `json:"reason"`
			Error  string `json:"error"`
		}
		_ = json.Unmarshal(args, &a)
		r := TaskFail(id, a.ID, a.Reason, a.Error)
		return marshal(r), r.Reason != ""
	case "task_delegate":
		var a struct {
			ID     string `json:"id"`
			Target string `json:"target"`
			Title  string `json:"title"`
			Body   string `json:"body"`
		}
		_ = json.Unmarshal(args, &a)
		r := TaskDelegate(id, a.ID, a.Target, a.Title, a.Body)
		return marshal(r), r.Reason != ""
	case "group_say":
		var a struct {
			Text string `json:"text"`
		}
		_ = json.Unmarshal(args, &a)
		r := GroupSay(id, a.Text)
		return marshal(r), r.Reason != ""
	case "group_pass":
		r := GroupPass(id)
		return marshal(r), r.Reason != ""
	default:
		return fmt.Sprintf(`{"reason":"missing_config","error":"unknown tool %s"}`, name), true
	}
}

func marshal(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return `{"reason":"unknown"}`
	}
	return string(b)
}

func IdentityFromEnv() Identity {
	cwd, _ := os.Getwd()
	return Identity{
		Root:    os.Getenv("CRUSHBOT_HOME"),
		Bot:     os.Getenv("CRUSHBOT_BOT"),
		DataDir: os.Getenv("CRUSHBOT_DATA_DIR"),
		Cwd:     cwd,
		Token:   os.Getenv("CRUSHBOT_MCP_TOKEN"),
	}
}

package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/evanxdsouza/wackcluborchard/internal/runtime"
	"github.com/evanxdsouza/wackcluborchard/internal/store"
)

type SandboxInput struct {
	Name      string          `json:"name"`
	Image     string          `json:"image"`
	Repo      string          `json:"repo"`
	Resources store.Resources `json:"resources"`
	StorageGi int             `json:"storageGi"`
}

func (p *Platform) sandboxNS(d *store.Data, orgID string) string {
	return "wackcluborchard-" + d.Orgs[orgID].Slug + "-greenhouse"
}

func (p *Platform) CreateSandbox(user *store.User, orgID string, in SandboxInput) (*store.Sandbox, error) {
	in.Name = strings.ToLower(strings.TrimSpace(in.Name))
	if err := ValidName(in.Name); err != nil {
		return nil, err
	}
	if in.Resources.CPUMillis == 0 {
		in.Resources = store.Resources{CPUMillis: 2000, MemoryMi: 4096}
	}
	if in.StorageGi == 0 {
		in.StorageGi = 20
	}
	var sb *store.Sandbox
	err := p.Store.Write(func(d *store.Data) error {
		if d.Orgs[orgID] == nil {
			return Invalid("organization not found")
		}
		for _, x := range d.Sandboxes {
			if x.OrgID == orgID && x.Name == in.Name {
				return Invalid("a sandbox named %q already exists", in.Name)
			}
		}
		if err := checkQuota(d, orgID, user.ID, Usage{Sandboxes: 1, CPUMillis: in.Resources.CPUMillis, MemoryMi: in.Resources.MemoryMi, StorageGi: in.StorageGi}); err != nil {
			return err
		}
		sb = &store.Sandbox{ID: store.NewID("sbx"), OrgID: orgID, OwnerID: user.ID, Name: in.Name, Image: in.Image, Repo: in.Repo,
			Status: "booting", BootProgress: 0, BootStage: "Queued", Resources: in.Resources, StorageGi: in.StorageGi, CreatedAt: time.Now()}
		d.Sandboxes[sb.ID] = sb
		sb = store.Clone(sb)
		return nil
	})
	if err != nil {
		return nil, err
	}
	p.enqueueSandbox(sb.ID)
	return sb, nil
}

func (p *Platform) sandboxSpec(id string) (runtime.SandboxSpec, bool) {
	var s runtime.SandboxSpec
	ok := false
	p.Store.Read(func(d *store.Data) {
		sb := d.Sandboxes[id]
		if sb == nil {
			return
		}
		s = runtime.SandboxSpec{Namespace: p.sandboxNS(d, sb.OrgID), Name: sb.Name, ID: sb.ID, Image: sb.Image, Repo: sb.Repo,
			Resources: sb.Resources, StorageGi: sb.StorageGi, RuntimeClass: "kata"}
		if s.Image == "" {
			s.Image = "mcr.microsoft.com/devcontainers/universal:2-linux"
		}
		if os.Getenv("SANDBOX_RUNTIME_CLASS") != "" {
			s.RuntimeClass = os.Getenv("SANDBOX_RUNTIME_CLASS")
		}
		ok = true
	})
	return s, ok
}

func (p *Platform) enqueueSandbox(id string) {
	p.queue.enqueue("sbx:"+id, func(ctx context.Context) error {
		s, ok := p.sandboxSpec(id)
		if !ok {
			return nil
		}
		if err := p.Driver.EnsureNamespace(ctx, s.Namespace, nil); err != nil {
			return err
		}
		return p.Driver.ApplySandbox(ctx, s)
	})
}

func (p *Platform) RestartSandbox(id string) {
	p.SandboxState(id, "booting", 0, "Queued")
	p.enqueueSandbox(id)
}

func (p *Platform) DeleteSandbox(id string) error {
	var ns, name string
	err := p.Store.Write(func(d *store.Data) error {
		sb := d.Sandboxes[id]
		if sb == nil {
			return Invalid("sandbox not found")
		}
		ns, name = p.sandboxNS(d, sb.OrgID), sb.Name
		delete(d.Sandboxes, id)
		return nil
	})
	if err != nil {
		return err
	}
	p.queue.enqueue("sbx:"+id, func(ctx context.Context) error { return p.Driver.DeleteSandbox(ctx, ns, name) })
	return nil
}

// SandboxRun runs a non-interactive command and returns its output.
func (p *Platform) SandboxRun(ctx context.Context, id string, cmd []string, stdin string) (string, error) {
	s, ok := p.sandboxSpec(id)
	if !ok {
		return "", Invalid("sandbox not found")
	}
	var out bytes.Buffer
	var in io.Reader
	if stdin != "" || (len(cmd) > 0 && cmd[0] == "wackcluborchard-write") {
		in = strings.NewReader(stdin)
	}
	err := p.Driver.SandboxExec(ctx, s.Namespace, s.Name, cmd, runtime.Stdio{Stdin: in, Stdout: &out})
	return out.String(), err
}

func (p *Platform) SandboxShell(ctx context.Context, id string, io runtime.Stdio) error {
	s, ok := p.sandboxSpec(id)
	if !ok {
		return Invalid("sandbox not found")
	}
	return p.Driver.SandboxExec(ctx, s.Namespace, s.Name, nil, io)
}

// ---- the agent ----

// AgentMessage runs one user turn of the in-sandbox agent: Claude with
// tools that read, write and run things in the sandbox's workspace.
// Messages are appended to the conversation and published as they happen.
func (p *Platform) AgentMessage(sandboxID, convID, text string) error {
	var hist []store.Message
	var cwd string
	err := p.Store.Write(func(d *store.Data) error {
		sb := d.Sandboxes[sandboxID]
		if sb == nil {
			return Invalid("sandbox not found")
		}
		for i := range sb.Conversations {
			c := &sb.Conversations[i]
			if c.ID == convID {
				c.Messages = append(c.Messages, store.Message{Role: "user", Text: text, CreatedAt: time.Now()})
				if c.Title == "" || c.Title == "New conversation" {
					c.Title = firstLine(text, 48)
				}
				hist = append([]store.Message(nil), c.Messages...)
				cwd = c.Cwd
				return nil
			}
		}
		return Invalid("conversation not found")
	})
	if err != nil {
		return err
	}
	p.publishConv(sandboxID, convID)
	go p.agentLoop(sandboxID, convID, cwd, hist)
	return nil
}

func firstLine(s string, n int) string {
	s = strings.TrimSpace(strings.SplitN(s, "\n", 2)[0])
	if len(s) > n {
		s = s[:n] + "…"
	}
	return s
}

func (p *Platform) appendMsg(sandboxID, convID, role, text string) {
	p.Store.Write(func(d *store.Data) error {
		if sb := d.Sandboxes[sandboxID]; sb != nil {
			for i := range sb.Conversations {
				if sb.Conversations[i].ID == convID {
					sb.Conversations[i].Messages = append(sb.Conversations[i].Messages, store.Message{Role: role, Text: text, CreatedAt: time.Now()})
				}
			}
		}
		return nil
	})
	p.publishConv(sandboxID, convID)
}

func (p *Platform) publishConv(sandboxID, convID string) {
	var c *store.Conversation
	p.Store.Read(func(d *store.Data) {
		if sb := d.Sandboxes[sandboxID]; sb != nil {
			for i := range sb.Conversations {
				if sb.Conversations[i].ID == convID {
					c = store.Clone(&sb.Conversations[i])
				}
			}
		}
	})
	if c != nil {
		p.Bus.Publish("sandbox:"+sandboxID, "conversation.updated", c)
	}
}

var agentTools = []map[string]any{
	{"name": "list_files", "description": "List files in the workspace.", "input_schema": map[string]any{"type": "object", "properties": map[string]any{}}},
	{"name": "read_file", "description": "Read a file from the workspace.", "input_schema": map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}, "required": []string{"path"}}},
	{"name": "write_file", "description": "Create or overwrite a file in the workspace.", "input_schema": map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}, "content": map[string]any{"type": "string"}}, "required": []string{"path", "content"}}},
	{"name": "run", "description": "Run a shell command in the workspace and return its combined output.", "input_schema": map[string]any{"type": "object", "properties": map[string]any{"command": map[string]any{"type": "string"}}, "required": []string{"command"}}},
}

func (p *Platform) agentLoop(sandboxID, convID, cwd string, hist []store.Message) {
	key := p.Settings().AnthropicKey
	if key == "" {
		key = os.Getenv("ANTHROPIC_API_KEY")
	}
	if key == "" {
		p.appendMsg(sandboxID, convID, "assistant", "The agent needs an Anthropic API key. An instance admin can add one under Instance admin → Settings.")
		return
	}
	model := os.Getenv("WACKCLUBORCHARD_AGENT_MODEL")
	if model == "" {
		model = "claude-sonnet-5"
	}
	var msgs []map[string]any
	for _, m := range hist {
		if m.Role == "user" || m.Role == "assistant" {
			msgs = append(msgs, map[string]any{"role": m.Role, "content": m.Text})
		}
	}
	system := "You are a coding agent working inside a Wack Club Orchard greenhouse sandbox. The workspace is at /workspace"
	if cwd != "" {
		system += ", and this conversation works in " + cwd
	}
	system += ". Use the tools to inspect and change files and run commands. Be concise."
	ctx, cancel := context.WithTimeout(p.ctx, 15*time.Minute)
	defer cancel()
	for turn := 0; turn < 25; turn++ {
		body, _ := json.Marshal(map[string]any{"model": model, "max_tokens": 4096, "system": system, "tools": agentTools, "messages": msgs})
		req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
		req.Header.Set("x-api-key", key)
		req.Header.Set("anthropic-version", "2023-06-01")
		req.Header.Set("content-type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			p.appendMsg(sandboxID, convID, "error", err.Error())
			return
		}
		var out struct {
			Content []struct {
				Type  string          `json:"type"`
				Text  string          `json:"text"`
				ID    string          `json:"id"`
				Name  string          `json:"name"`
				Input json.RawMessage `json:"input"`
			} `json:"content"`
			StopReason string `json:"stop_reason"`
			Error      *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&out)
		resp.Body.Close()
		if out.Error != nil {
			p.appendMsg(sandboxID, convID, "error", out.Error.Message)
			return
		}
		var assistant []any
		var results []any
		for _, c := range out.Content {
			switch c.Type {
			case "text":
				assistant = append(assistant, map[string]any{"type": "text", "text": c.Text})
				if strings.TrimSpace(c.Text) != "" {
					p.appendMsg(sandboxID, convID, "assistant", c.Text)
				}
			case "tool_use":
				assistant = append(assistant, map[string]any{"type": "tool_use", "id": c.ID, "name": c.Name, "input": c.Input})
				var in map[string]string
				json.Unmarshal(c.Input, &in)
				res := p.agentTool(ctx, sandboxID, cwd, c.Name, in)
				p.appendMsg(sandboxID, convID, "tool", fmt.Sprintf("%s %s\n%s", c.Name, toolArg(in), trunc(res, 4000)))
				results = append(results, map[string]any{"type": "tool_result", "tool_use_id": c.ID, "content": trunc(res, 30000)})
			}
		}
		msgs = append(msgs, map[string]any{"role": "assistant", "content": assistant})
		if out.StopReason != "tool_use" || len(results) == 0 {
			return
		}
		msgs = append(msgs, map[string]any{"role": "user", "content": results})
	}
}

func toolArg(in map[string]string) string {
	if v := in["command"]; v != "" {
		return v
	}
	return in["path"]
}

func trunc(s string, n int) string {
	if len(s) > n {
		return s[:n] + "\n…(truncated)"
	}
	return s
}

func (p *Platform) agentTool(ctx context.Context, sandboxID, cwd, name string, in map[string]string) string {
	var out string
	var err error
	prefix := ""
	if cwd != "" {
		prefix = strings.Trim(cwd, "/") + "/"
	}
	switch name {
	case "list_files":
		out, err = p.SandboxRun(ctx, sandboxID, []string{"wackcluborchard-ls"}, "")
	case "read_file":
		out, err = p.SandboxRun(ctx, sandboxID, []string{"wackcluborchard-read", prefix + in["path"]}, "")
	case "write_file":
		_, err = p.SandboxRun(ctx, sandboxID, []string{"wackcluborchard-write", prefix + in["path"]}, in["content"])
		out = "wrote " + in["path"]
	case "run":
		c := in["command"]
		if cwd != "" {
			c = "cd /workspace/" + strings.Trim(cwd, "/") + " && " + c
		} else {
			c = "cd /workspace && " + c
		}
		out, err = p.SandboxRun(ctx, sandboxID, []string{"sh", "-c", c + " 2>&1"}, "")
	default:
		return "unknown tool"
	}
	if err != nil {
		return out + "\nerror: " + err.Error()
	}
	return out
}

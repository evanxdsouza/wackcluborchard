package api

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/evanxdsouza/wackcluborchard/internal/store"
)

// AdminSocket serves instance-operator commands on a unix socket inside
// the data directory. Only someone with access to the box (or `kubectl
// exec` into the server pod) can reach it, which is exactly who should be
// able to mint a recovery claim link.
func (s *Server) AdminSocket(path string) error {
	os.Remove(path)
	l, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	os.Chmod(path, 0o600)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /claim", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			URL string `json:"url"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		base := in.URL
		if base == "" {
			base = s.Cfg.FrontendURL
		}
		if base == "" {
			s.P.Store.Read(func(d *store.Data) { base = "http://" + d.Settings.Domain })
		}
		if !strings.Contains(base, "://") {
			base = "http://" + base
		}
		tok := MintRecoveryToken(s.P.Store)
		writeJSON(w, 200, map[string]string{"url": strings.TrimRight(base, "/") + "/claim?token=" + tok})
	})
	mux.HandleFunc("POST /settings", func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		json.NewDecoder(r.Body).Decode(&in)
		s.P.Store.Write(func(d *store.Data) error {
			st := &d.Settings
			if v, ok := in["publicIp"].(string); ok {
				st.PublicIP = v
			}
			if v, ok := in["mcpEnabled"].(bool); ok {
				st.MCPEnabled = v
			}
			if v, ok := in["mcpDomain"].(string); ok {
				st.MCPDomain = v
			}
			if v, ok := in["signupMode"].(string); ok && (v == "open" || v == "invite" || v == "closed") {
				st.SignupMode = v
			}
			if v, ok := in["addHostname"].(string); ok && v != "" {
				for _, h := range st.Hostnames {
					if h == v {
						return nil
					}
				}
				st.Hostnames = append(st.Hostnames, v)
			}
			if v, ok := in["removeHostname"].(string); ok {
				var keep []string
				for _, h := range st.Hostnames {
					if h != v {
						keep = append(keep, h)
					}
				}
				st.Hostnames = keep
			}
			return nil
		})
		s.P.Audit("", nil, "instance.settings_updated", "wackcluborchardctl", nil)
		var st store.Settings
		s.P.Store.Read(func(d *store.Data) { st = d.Settings })
		writeJSON(w, 200, map[string]any{"publicIp": st.PublicIP, "mcpEnabled": st.MCPEnabled, "mcpDomain": st.MCPDomain, "hostnames": st.Hostnames, "signupMode": st.SignupMode})
	})
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		out := map[string]any{"runtime": s.P.Driver.Name(), "version": s.P.Cfg.Version}
		s.P.Store.Read(func(d *store.Data) {
			out["users"] = len(d.Users)
			out["orgs"] = len(d.Orgs)
			out["apps"] = len(d.Apps)
			out["databases"] = len(d.Databases)
			out["domain"] = d.Settings.Domain
			out["appDomain"] = d.Settings.AppDomain
			out["publicIp"] = d.Settings.PublicIP
			out["mcpEnabled"] = d.Settings.MCPEnabled
			claimed := false
			for _, u := range d.Users {
				if u.Superadmin {
					claimed = true
				}
			}
			out["claimed"] = claimed
		})
		run, wait, slots := s.P.BuildStats()
		out["builds"] = map[string]int{"running": run, "waiting": wait, "slots": slots}
		writeJSON(w, 200, out)
	})
	go http.Serve(l, mux)
	return nil
}

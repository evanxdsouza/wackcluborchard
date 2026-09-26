// Command orchard-server runs the Wack Club Orchard control plane: API,
// dashboard, MCP endpoint, build queue, job scheduler and informers.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/evanxdsouza/wackcluborchard/internal/api"
	"github.com/evanxdsouza/wackcluborchard/internal/github"
	"github.com/evanxdsouza/wackcluborchard/internal/kube"
	"github.com/evanxdsouza/wackcluborchard/internal/platform"
	"github.com/evanxdsouza/wackcluborchard/internal/runtime"
	"github.com/evanxdsouza/wackcluborchard/internal/store"
	"github.com/evanxdsouza/wackcluborchard/web"
)

var version = "dev"

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func main() {
	addr := flag.String("addr", env("ORCHARD_ADDR", ":"+env("PORT", "8080")), "listen address")
	dataDir := flag.String("data", env("ORCHARD_DATA", "./data"), "state directory")
	rt := flag.String("runtime", env("ORCHARD_RUNTIME", "auto"), "runtime: auto, kubernetes or sim")
	demo := flag.Bool("demo", env("ORCHARD_DEMO", "") == "true", "seed a demo project for the first user")
	webDir := flag.String("web", env("ORCHARD_WEB_DIR", ""), "serve the frontend from this directory instead of the embedded build")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(filepath.Join(*dataDir, "orchard.json"))
	if err != nil {
		log.Fatalf("open state: %v", err)
	}
	defer st.Close()
	initSettings(st)

	driver, err := pickRuntime(*rt)
	if err != nil {
		log.Fatal(err)
	}
	if driver.Name() == "sim" && !*demo && os.Getenv("ORCHARD_DEMO") == "" {
		*demo = true
	}
	frontend := strings.TrimRight(env("FRONTEND_URL", ""), "/")
	p := platform.New(ctx, st, driver, platform.Config{FrontendURL: frontend, Registry: env("REGISTRY_HOST", ""), Version: version})
	p.CloneToken = func(ctx context.Context, repo string) string {
		cfg := p.Settings().GitHub
		if cfg.PrivateKey == "" {
			return ""
		}
		t, err := github.InstallationToken(ctx, cfg, repo)
		if err != nil {
			log.Printf("github installation token for %s: %v", repo, err)
		}
		return t
	}
	if err := p.Start(); err != nil {
		log.Fatalf("start runtime: %v", err)
	}

	var webFS fs.FS
	if *webDir != "" {
		webFS = os.DirFS(*webDir)
	} else if sub, err := fs.Sub(web.Dist, "dist"); err == nil {
		webFS = sub
	}
	srv := api.New(p, api.Config{
		FrontendURL:  frontend,
		SecureCookie: strings.HasPrefix(frontend, "https://"),
		Web:          webFS,
		Demo:         *demo,
	})

	if tok, minted := api.MintSetupToken(st); minted {
		base := frontend
		if base == "" {
			base = "http://localhost" + *addr
			if strings.HasPrefix(*addr, ":") {
				base = "http://localhost" + *addr
			}
		}
		log.Printf("no superadmin yet. Sign up, then open this single-use link (valid 24h) to claim the instance:\n\n    %s/claim?token=%s\n", base, tok)
	}

	hs := &http.Server{Addr: *addr, Handler: srv, ReadHeaderTimeout: 15 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		hs.Shutdown(sctx)
	}()
	log.Printf("Wack Club Orchard %s listening on %s (runtime: %s)", version, *addr, driver.Name())
	if err := hs.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func pickRuntime(name string) (runtime.Driver, error) {
	switch name {
	case "sim":
		return runtime.NewSim(), nil
	case "kubernetes", "kube", "k8s":
		c, err := kube.Config()
		if err != nil {
			return nil, fmt.Errorf("kubernetes config: %w", err)
		}
		return runtime.NewKube(c, runtime.DefaultKubeConfig()), nil
	case "auto":
		if c, err := kube.Config(); err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if _, _, err := c.List(ctx, "Namespace", "", kube.ListOpts{LabelSelector: "kubernetes.io/metadata.name=default"}); err == nil {
				return runtime.NewKube(c, runtime.DefaultKubeConfig()), nil
			}
		}
		log.Printf("no Kubernetes cluster reachable; using the simulated runtime")
		return runtime.NewSim(), nil
	}
	return nil, fmt.Errorf("unknown runtime %q", name)
}

// initSettings applies environment configuration. Values set in the
// environment win over stored ones so the installer's values file stays
// the source of truth.
func initSettings(st *store.Store) {
	st.Write(func(d *store.Data) error {
		s := &d.Settings
		set := func(dst *string, key, def string) {
			if v := os.Getenv(key); v != "" {
				*dst = v
			} else if *dst == "" {
				*dst = def
			}
		}
		set(&s.InstanceName, "INSTANCE_NAME", "Wack Club Orchard")
		set(&s.Domain, "ORCHARD_DOMAIN", "localhost")
		set(&s.AppDomain, "APP_DOMAIN", "apps.localhost")
		set(&s.MCPDomain, "MCP_DOMAIN", "")
		set(&s.IngressMode, "INGRESS_MODE", "lan")
		set(&s.IngressCNAME, "INGRESS_CNAME", "")
		set(&s.PublicIP, "PUBLIC_IP", "")
		set(&s.PublicDBDomain, "PUBLIC_DB_DOMAIN", "")
		set(&s.SignupMode, "SIGNUP_MODE", "open")
		if v := os.Getenv("MCP_ENABLED"); v != "" {
			s.MCPEnabled = v == "true"
		} else if !s.MCPEnabled && s.MCPDomain == "" {
			s.MCPEnabled = true
		}
		if n, err := strconv.Atoi(os.Getenv("BUILD_SLOTS")); err == nil && n > 0 {
			s.BuildSlots = n
		} else if s.BuildSlots == 0 {
			s.BuildSlots = 2
		}
		if n, err := strconv.Atoi(os.Getenv("PUBLIC_HTTP_PORT")); err == nil {
			s.HTTPPort = n
		}
		if n, err := strconv.Atoi(os.Getenv("PUBLIC_HTTPS_PORT")); err == nil {
			s.HTTPSPort = n
		}
		if os.Getenv("TENANT_SANDBOX") == "true" {
			s.TenantSandbox = true
		}
		if os.Getenv("BUILDER_SANDBOX") == "true" {
			s.BuilderSandbox = true
		}
		if s.Secret == "" {
			s.Secret = platform.RandHex(32)
		}
		return nil
	})
}

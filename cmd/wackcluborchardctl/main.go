// Command wackcluborchardctl operates a Wack Club Orchard install from the box it
// runs on: mint claim links, set the public IP, add hostnames, toggle the
// MCP endpoint, and apply /etc/wackcluborchard/values.yaml with Helm.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

var version = "dev"

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

var (
	namespace  = env("WACKCLUBORCHARD_NAMESPACE", "wackcluborchard")
	deployment = env("WACKCLUBORCHARD_DEPLOYMENT", "wackcluborchard-server")
	values     = env("WACKCLUBORCHARD_VALUES", "/etc/wackcluborchard/values.yaml")
	chart      = env("WACKCLUBORCHARD_CHART", "/usr/local/share/wackcluborchard/chart")
	release    = env("WACKCLUBORCHARD_RELEASE", "wackcluborchard")
)

const usage = `wackcluborchardctl: operate this Wack Club Orchard instance

  wackcluborchardctl status                      instance summary
  wackcluborchardctl claim [--url host]          mint a single-use claim / recovery link
  wackcluborchardctl public-ip set <ip> [--apply]
  wackcluborchardctl hostname add <name> [--apply]
  wackcluborchardctl hostname remove <name> [--apply]
  wackcluborchardctl mcp enable|disable [--domain d] [--apply]
  wackcluborchardctl signup open|invite|closed
  wackcluborchardctl config get <path>           read /etc/wackcluborchard/values.yaml (needs yq)
  wackcluborchardctl config set <path> <value>   write it
  wackcluborchardctl update                      helm upgrade with the values file
  wackcluborchardctl logs [-f]                   control-plane logs

--apply also writes the change to the values file and runs an update, so
it survives the next upgrade.
`

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
	return cmd.Run()
}

func output(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func kubectl() string {
	if _, err := exec.LookPath("kubectl"); err == nil {
		return "kubectl"
	}
	if _, err := exec.LookPath("k3s"); err == nil {
		return "k3s kubectl"
	}
	fmt.Fprintln(os.Stderr, "wackcluborchardctl: kubectl not found")
	os.Exit(1)
	return ""
}

// admin runs `wackcluborchard-server admin ...` inside the server pod.
func admin(args ...string) (string, error) {
	k := strings.Fields(kubectl())
	full := append(k[1:], "-n", namespace, "exec", "deploy/"+deployment, "--", "wackcluborchard-server", "admin")
	full = append(full, args...)
	full = append(full, "--data", "/data")
	return output(k[0], full...)
}

func die(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "wackcluborchardctl: "+format+"\n", a...)
	os.Exit(1)
}

func has(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

func flagValue(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(a, flag+"=") {
			return strings.TrimPrefix(a, flag+"=")
		}
	}
	return ""
}

func yq(expr string) error {
	if _, err := exec.LookPath("yq"); err != nil {
		return fmt.Errorf("yq is not installed; edit %s by hand", values)
	}
	return run("yq", "-i", expr, values)
}

func update() {
	if _, err := os.Stat(values); err != nil {
		die("no values file at %s", values)
	}
	fmt.Println("applying", values)
	if err := run("helm", "upgrade", "--install", release, chart, "-n", namespace, "--create-namespace", "-f", values, "--wait", "--timeout", "10m"); err != nil {
		die("helm upgrade failed: %v", err)
	}
}

func settings(kv ...string) {
	out, err := admin(append([]string{"set"}, kv...)...)
	if err != nil {
		die("%v\n%s", err, out)
	}
	fmt.Println(out)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		return
	}
	args := os.Args[2:]
	switch os.Args[1] {
	case "version":
		fmt.Println("wackcluborchardctl", version)
	case "status":
		out, err := admin("status")
		if err != nil {
			die("%v\n%s", err, out)
		}
		fmt.Println(out)
	case "claim":
		a := []string{"claim"}
		if u := flagValue(args, "--url"); u != "" {
			if !strings.Contains(u, "://") {
				u = "http://" + u
			}
			a = append(a, "--url", u)
		}
		out, err := admin(a...)
		if err != nil {
			die("%v\n%s", err, out)
		}
		fmt.Println("Single-use claim link (valid 24h). It signs in as the instance superadmin; open it yourself:")
		fmt.Println()
		fmt.Println("    " + out)
	case "public-ip":
		if len(args) < 2 || args[0] != "set" {
			die("usage: wackcluborchardctl public-ip set <ip> [--apply]")
		}
		settings("publicIp=" + args[1])
		if has(args, "--apply") {
			if err := yq(fmt.Sprintf(`(.server.env[] | select(.name == "PUBLIC_IP")).value = "%s"`, args[1])); err != nil {
				die("%v", err)
			}
			update()
		}
	case "hostname":
		if len(args) < 2 {
			die("usage: wackcluborchardctl hostname add|remove <name> [--apply]")
		}
		switch args[0] {
		case "add":
			settings("addHostname=" + args[1])
			if has(args, "--apply") {
				if err := yq(fmt.Sprintf(`.ingress.extraHosts += ["%s"] | .ingress.extraHosts |= unique`, args[1])); err != nil {
					die("%v", err)
				}
				update()
			}
		case "remove":
			settings("removeHostname=" + args[1])
			if has(args, "--apply") {
				if err := yq(fmt.Sprintf(`.ingress.extraHosts -= ["%s"]`, args[1])); err != nil {
					die("%v", err)
				}
				update()
			}
		default:
			die("usage: wackcluborchardctl hostname add|remove <name>")
		}
	case "mcp":
		if len(args) < 1 {
			die("usage: wackcluborchardctl mcp enable|disable [--domain d] [--apply]")
		}
		on := args[0] == "enable"
		kv := []string{fmt.Sprintf("mcpEnabled=%v", on)}
		if d := flagValue(args, "--domain"); d != "" {
			kv = append(kv, "mcpDomain="+d)
		}
		settings(kv...)
		if has(args, "--apply") {
			expr := fmt.Sprintf(`.mcp.enabled = %v`, on)
			if d := flagValue(args, "--domain"); d != "" {
				expr += fmt.Sprintf(` | .mcp.host = "%s"`, d)
			}
			if err := yq(expr); err != nil {
				die("%v", err)
			}
			update()
		}
	case "signup":
		if len(args) < 1 {
			die("usage: wackcluborchardctl signup open|invite|closed")
		}
		settings("signupMode=" + args[0])
	case "config":
		if len(args) < 2 {
			die("usage: wackcluborchardctl config get|set <path> [value]")
		}
		path := args[1]
		if !strings.HasPrefix(path, ".") {
			path = "." + path
		}
		switch args[0] {
		case "get":
			if err := run("yq", path, values); err != nil {
				die("%v", err)
			}
		case "set":
			if len(args) < 3 {
				die("usage: wackcluborchardctl config set <path> <value>")
			}
			if err := yq(fmt.Sprintf(`%s = "%s"`, path, args[2])); err != nil {
				die("%v", err)
			}
			fmt.Println("saved; run `wackcluborchardctl update` to apply")
		}
	case "update":
		update()
	case "logs":
		k := strings.Fields(kubectl())
		a := append(k[1:], "-n", namespace, "logs", "deploy/"+deployment, "--tail", "200")
		if has(args, "-f") {
			a = append(a, "-f")
		}
		run(k[0], a...)
	default:
		fmt.Print(usage)
		os.Exit(2)
	}
}

package main

import (
	"fmt"
	"strings"

	"github.com/FalconTechDev/synchronize/internal/syn"
)

// cmdDeploy turns a repository into somewhere code is deployed to rather
// than worked in. It is the one setting that separates a server from a
// laptop, so it gets its own verb instead of hiding in `syn set`.
func cmdDeploy(cfg *syn.Config, rest []string) error {
	if len(rest) == 0 {
		return showDeploy(cfg)
	}
	r := cfg.Find(rest[0])
	if r == nil {
		return fmt.Errorf("unknown repository %q\n%s", rest[0], repoListHint(cfg))
	}
	if len(rest) == 1 {
		if r.PullOnly {
			fmt.Printf("%s is a deploy target: changes come down, nothing goes up.\n", r.Name)
		} else {
			fmt.Printf("%s syncs both ways.\n", r.Name)
		}
		return nil
	}

	switch rest[1] {
	case "on", "true", "yes":
		r.PullOnly = true
	case "off", "false", "no":
		r.PullOnly = false
	default:
		return fmt.Errorf("deploy is on or off, not %q", rest[1])
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	if r.PullOnly {
		syn.Info("%s: deploy target - syncing brings changes down and sends nothing up", r.Name)
		// Whatever the service writes into that folder now stays local. Say
		// so, because the usual promise of syn is the opposite.
		syn.Info("       anything written there stays on this machine, uncommitted")
	} else {
		syn.Info("%s: syncs both ways again", r.Name)
	}
	return nil
}

func showDeploy(cfg *syn.Config) error {
	var targets []string
	for i := range cfg.Repos {
		if cfg.Repos[i].PullOnly {
			targets = append(targets, cfg.Repos[i].Name)
		}
	}
	if len(targets) == 0 {
		fmt.Println("No repository is a deploy target - all of them sync both ways.")
		fmt.Println()
		fmt.Println("On a server, mark the repositories you deploy into:")
		fmt.Println()
		fmt.Println("  syn deploy my-bot on")
		fmt.Println()
		fmt.Println("Then syncing only ever brings changes down, and whatever the")
		fmt.Println("service writes into that folder is never committed or published.")
		return nil
	}
	fmt.Println("Deploy targets (changes come down, nothing goes up):")
	for _, name := range targets {
		fmt.Printf("  %s\n", name)
	}
	return nil
}

// cmdService installs the watcher as a system service - the difference
// between a server and a desktop being that it has to run without anybody
// logging in.
func cmdService(cfg *syn.Config, rest []string, values map[string]string) error {
	what := ""
	if len(rest) > 0 {
		what = rest[0]
	}
	switch what {
	case "", "status":
		fmt.Printf("system service: %s\n", syn.ServiceStatus())
		if !syn.ServiceSupported() {
			return nil
		}
		fmt.Println()
		fmt.Println("  install:  sudo syn service install --as <user>")
		fmt.Println("  logs:     journalctl -u synchronize.service -f")
		return nil

	case "install":
		if err := syn.InstallService(syn.ServiceOptions{
			User: values["--as"],
			Home: values["--home"],
		}); err != nil {
			return err
		}
		if deployHint(cfg) != "" {
			fmt.Println()
			fmt.Println(deployHint(cfg))
		}
		return nil

	case "uninstall", "remove":
		return syn.UninstallService()
	}
	return fmt.Errorf("unknown service subcommand %q (try: install, uninstall, status)", what)
}

// deployHint nudges once, at the moment it matters: a service was just
// installed on a machine where every repository still syncs both ways, which
// on a server means the running application's scratch files get published.
func deployHint(cfg *syn.Config) string {
	var both []string
	for i := range cfg.Repos {
		if !cfg.Repos[i].PullOnly {
			both = append(both, cfg.Repos[i].Name)
		}
	}
	if len(both) == 0 {
		return ""
	}
	return "These still sync both ways: " + strings.Join(both, ", ") + "\n" +
		"If this machine only runs the code rather than being worked on, say so -\n" +
		"otherwise logs and caches it writes will be committed and pushed:\n" +
		"  syn deploy " + both[0] + " on"
}

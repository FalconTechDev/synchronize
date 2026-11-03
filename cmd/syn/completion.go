package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Shell completion: `syn completion bash|zsh|fish|powershell`.
//
// With thirty-odd commands, subcommands under several of them, and repository
// names that you have to type exactly, completion saves more keystrokes here
// than in most tools. The scripts are generated from the catalogue the help
// and the hints under the shell prompt already use, so a command added there
// is completable without anyone remembering to edit a script.

// takesRepo are the commands whose argument is a repository name.
var takesRepo = map[string]bool{
	"pull": true, "push": true, "sync": true, "status": true, "diff": true,
	"undo": true, "resolve": true, "backups": true, "restore": true,
	"prune": true, "set": true, "files": true, "ignore": true, "remove": true,
	"enable": true, "disable": true, "path": true, "pause": true, "deploy": true,
	"app": true, "restart": true, "watch": true, "hook": true, "log": true,
	"auto": true,
}

// completable are the commands worth completing at the shell: all of them,
// less the two that only exist inside `syn shell`, plus the two that are not in
// the catalogue because they need no hint.
func completable() []string {
	skip := map[string]bool{"clear": true, "exit": true}
	seen := map[string]bool{}
	var out []string
	for _, name := range append(knownCommands(), "help", "version") {
		if skip[name] || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

type completionData struct {
	Commands []string
	Subs     map[string][]string
	Flags    map[string][]string
}

var flagPattern = regexp.MustCompile(`--[a-z][a-z-]*`)

func buildCompletionData() completionData {
	d := completionData{
		Commands: completable(),
		Subs:     map[string][]string{},
		Flags:    map[string][]string{},
	}

	for key := range subHints {
		parts := strings.SplitN(key, " ", 2)
		if len(parts) == 2 {
			d.Subs[parts[0]] = append(d.Subs[parts[0]], parts[1])
		}
	}
	d.Subs["completion"] = []string{"bash", "fish", "powershell", "zsh"}
	for name := range d.Subs {
		sort.Strings(d.Subs[name])
	}

	seen := map[string]map[string]bool{}
	add := func(cmd, text string) {
		for _, f := range flagPattern.FindAllString(text, -1) {
			if seen[cmd] == nil {
				seen[cmd] = map[string]bool{}
			}
			if !seen[cmd][f] {
				seen[cmd][f] = true
				d.Flags[cmd] = append(d.Flags[cmd], f)
			}
		}
	}
	for name, info := range catalog {
		add(name, info.Args)
	}
	for key, info := range subHints {
		add(strings.SplitN(key, " ", 2)[0], info.Args)
	}
	for _, f := range d.Flags {
		sort.Strings(f)
	}
	return d
}

func (d completionData) words(list []string) string { return strings.Join(list, " ") }

// completionScript returns the script for one shell.
func completionScript(shell string) (string, error) {
	d := buildCompletionData()
	switch shell {
	case "bash":
		return d.bash(), nil
	case "zsh":
		return d.zsh(), nil
	case "fish":
		return d.fish(), nil
	case "powershell", "pwsh":
		return d.powershell(), nil
	}
	return "", fmt.Errorf("no completion for %q - pick one of: bash, zsh, fish, powershell", shell)
}

func (d completionData) repoCommands() []string {
	var out []string
	for _, c := range d.Commands {
		if takesRepo[c] {
			out = append(out, c)
		}
	}
	return out
}

// plainRepoCommands take a repository and have no subcommands to complete.
func (d completionData) plainRepoCommands() []string {
	var out []string
	for _, c := range d.repoCommands() {
		if _, has := d.Subs[c]; !has {
			out = append(out, c)
		}
	}
	return out
}

func (d completionData) bash() string {
	var b strings.Builder
	b.WriteString("# bash completion for syn.  Load it with:  source <(syn completion bash)\n")
	b.WriteString("_syn() {\n")
	b.WriteString("  local cur=\"${COMP_WORDS[COMP_CWORD]}\" cmd=\"${COMP_WORDS[1]}\"\n")
	b.WriteString("  COMPREPLY=()\n")
	fmt.Fprintf(&b, "  if [ \"$COMP_CWORD\" -eq 1 ]; then COMPREPLY=($(compgen -W %q -- \"$cur\")); return; fi\n",
		d.words(d.Commands))

	b.WriteString("  if [[ \"$cur\" == -* ]]; then\n    case \"$cmd\" in\n")
	for _, c := range sortedKeys(d.Flags) {
		fmt.Fprintf(&b, "      %s) COMPREPLY=($(compgen -W %q -- \"$cur\")) ;;\n", c, d.words(d.Flags[c]))
	}
	b.WriteString("    esac\n    return\n  fi\n")

	b.WriteString("  local repos=\"\"\n")
	fmt.Fprintf(&b, "  case \"$cmd\" in %s) repos=\"$(syn ls --names 2>/dev/null)\" ;; esac\n",
		strings.Join(d.repoCommands(), "|"))

	b.WriteString("  case \"$cmd\" in\n")
	for _, c := range sortedKeys(d.Subs) {
		fmt.Fprintf(&b, "    %s)\n      if [ \"$COMP_CWORD\" -eq 2 ]; then COMPREPLY=($(compgen -W \"%s $repos\" -- \"$cur\")); else COMPREPLY=($(compgen -W \"$repos\" -- \"$cur\")); fi ;;\n",
			c, d.words(d.Subs[c]))
	}
	fmt.Fprintf(&b, "    %s) COMPREPLY=($(compgen -W \"all $repos\" -- \"$cur\")) ;;\n  esac\n}\n",
		strings.Join(d.plainRepoCommands(), "|"))
	b.WriteString("complete -F _syn syn\n")
	return b.String()
}

func (d completionData) zsh() string {
	var b strings.Builder
	b.WriteString("#compdef syn\n# zsh completion for syn.  Put it on your fpath as _syn, or:  source <(syn completion zsh)\n")
	b.WriteString("_syn() {\n")
	b.WriteString("  local cur=\"$words[CURRENT]\" cmd=\"$words[2]\"\n")
	fmt.Fprintf(&b, "  if (( CURRENT == 2 )); then compadd -- %s; return; fi\n",
		d.words(d.Commands))
	b.WriteString("  if [[ \"$cur\" == -* ]]; then\n    case \"$cmd\" in\n")
	for _, c := range sortedKeys(d.Flags) {
		fmt.Fprintf(&b, "      %s) compadd -- %s ;;\n", c, d.words(d.Flags[c]))
	}
	b.WriteString("    esac\n    return\n  fi\n")
	b.WriteString("  local -a repos\n")
	fmt.Fprintf(&b, "  case \"$cmd\" in %s) repos=(${(f)\"$(syn ls --names 2>/dev/null)\"}) ;; esac\n",
		strings.Join(d.repoCommands(), "|"))
	b.WriteString("  case \"$cmd\" in\n")
	for _, c := range sortedKeys(d.Subs) {
		fmt.Fprintf(&b, "    %s) if (( CURRENT == 3 )); then compadd -- %s $repos; else compadd -- $repos; fi ;;\n",
			c, d.words(d.Subs[c]))
	}
	fmt.Fprintf(&b, "    %s) compadd -- all $repos ;;\n  esac\n}\n", strings.Join(d.plainRepoCommands(), "|"))
	b.WriteString("compdef _syn syn\n")
	return b.String()
}

func (d completionData) fish() string {
	var b strings.Builder
	b.WriteString("# fish completion for syn.  Load it with:  syn completion fish | source\n")
	b.WriteString("complete -c syn -f\n")
	fmt.Fprintf(&b, "complete -c syn -n '__fish_use_subcommand' -a '%s'\n",
		d.words(d.Commands))
	for _, c := range sortedKeys(d.Subs) {
		fmt.Fprintf(&b, "complete -c syn -n '__fish_seen_subcommand_from %s' -a '%s'\n", c, d.words(d.Subs[c]))
	}
	fmt.Fprintf(&b, "complete -c syn -n '__fish_seen_subcommand_from %s' -a '(syn ls --names 2>/dev/null) all'\n",
		d.words(d.repoCommands()))
	for _, c := range sortedKeys(d.Flags) {
		for _, f := range d.Flags[c] {
			fmt.Fprintf(&b, "complete -c syn -n '__fish_seen_subcommand_from %s' -l %s\n", c, strings.TrimPrefix(f, "--"))
		}
	}
	return b.String()
}

func (d completionData) powershell() string {
	var b strings.Builder
	b.WriteString("# PowerShell completion for syn.  Load it with:  syn completion powershell | Out-String | Invoke-Expression\n")
	b.WriteString("Register-ArgumentCompleter -Native -CommandName syn -ScriptBlock {\n")
	b.WriteString("  param($wordToComplete, $commandAst, $cursorPosition)\n")
	b.WriteString("  $words = @($commandAst.CommandElements | ForEach-Object { $_.ToString() })\n")
	b.WriteString("  $index = $words.Count\n  if ($wordToComplete -eq '') { $index++ }\n")
	fmt.Fprintf(&b, "  $commands = '%s' -split ' '\n", d.words(d.Commands))
	b.WriteString("  $pool = @()\n")
	b.WriteString("  if ($index -le 2) { $pool = $commands } else {\n    $cmd = $words[1]\n")
	b.WriteString("    if ($wordToComplete -like '-*') {\n      switch ($cmd) {\n")
	for _, c := range sortedKeys(d.Flags) {
		fmt.Fprintf(&b, "        '%s' { $pool = '%s' -split ' ' }\n", c, d.words(d.Flags[c]))
	}
	b.WriteString("      }\n    } else {\n")
	fmt.Fprintf(&b, "      $takesRepo = '%s' -split ' '\n", d.words(d.repoCommands()))
	b.WriteString("      if ($takesRepo -contains $cmd) { $pool = @('all') + @(syn ls --names 2>$null) }\n")
	b.WriteString("      if ($index -eq 3) {\n        switch ($cmd) {\n")
	for _, c := range sortedKeys(d.Subs) {
		fmt.Fprintf(&b, "          '%s' { $pool = '%s' -split ' ' }\n", c, d.words(d.Subs[c]))
	}
	b.WriteString("        }\n      }\n    }\n  }\n")
	b.WriteString("  $pool | Where-Object { $_ -like \"$wordToComplete*\" } | ForEach-Object {\n")
	b.WriteString("    [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)\n  }\n}\n")
	return b.String()
}

func sortedKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

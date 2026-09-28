package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"private-proxy/internal/routes"
)

// completeCommand is the hidden command the shell completion scripts call
// to get suggestions for the word under the cursor.
const completeCommand = "__complete"

// commandNames are the subcommands offered when completing the first word.
var commandNames = []string{
	"add", "remove", "edit", "enable", "disable",
	"list", "validate", "apply", "completion", "help",
}

var completionShells = []string{"bash", "zsh"}

// runComplete prints one suggestion per line for the command line given in
// args: every word typed after "proxyctl", the last one being the (possibly
// empty) word being completed. It never fails loudly, so a broken
// routes.yaml only loses route suggestions instead of spamming the prompt.
func runComplete(root string, args []string) error {
	if len(args) == 0 {
		args = []string{""}
	}
	// Read-only, like list, so no lock needed.
	s, err := routes.Load(filepath.Join(root, routesFileName))
	if err != nil {
		s = nil
	}
	for _, c := range completions(s, args) {
		fmt.Println(c)
	}
	return nil
}

// completions returns the suggestions for the last word in words, filtered
// by that word as a prefix. s may be nil when routes.yaml couldn't be loaded.
func completions(s *routes.Store, words []string) []string {
	cur := words[len(words)-1]
	pos := len(words) - 1 // index of the argument being completed; 0 is the command

	var candidates []string
	if pos == 0 {
		candidates = commandNames
	} else {
		switch cmd := words[0]; {
		case cmd == "completion" && pos == 1:
			candidates = completionShells
		case (cmd == "remove" || cmd == "edit") && pos == 1:
			candidates = routeNames(s, func(routes.Route) bool { return true })
		case cmd == "enable" && pos == 1:
			candidates = routeNames(s, func(r routes.Route) bool { return !r.Enabled })
		case cmd == "disable" && pos == 1:
			candidates = routeNames(s, func(r routes.Route) bool { return r.Enabled })
		case cmd == "edit" && pos == 2 && s != nil:
			// Offer the current upstream, so changing just the port is quick.
			hostname, path := splitHostPath(words[1])
			if r, ok := s.Find(hostname, path); ok {
				candidates = []string{r.Upstream}
			}
		}
	}

	var out []string
	for _, c := range candidates {
		if strings.HasPrefix(c, cur) {
			out = append(out, c)
		}
	}
	return out
}

// routeNames returns the CLI tokens (hostname[/path]) of the routes that
// match keep, sorted.
func routeNames(s *routes.Store, keep func(routes.Route) bool) []string {
	if s == nil {
		return nil
	}
	var names []string
	for _, r := range s.Routes {
		if keep(r) {
			names = append(names, r.Hostname+r.Path)
		}
	}
	sort.Strings(names)
	return names
}

func runCompletion(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: proxyctl completion bash|zsh")
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate proxyctl executable: %w", err)
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return fmt.Errorf("resolve proxyctl executable path: %w", err)
	}

	// The scripts call the binary by its absolute path, so completion keeps
	// working even if proxyctl is only reachable through an alias.
	var script string
	switch args[0] {
	case "bash":
		script = bashCompletion
	case "zsh":
		script = zshCompletion
	default:
		return fmt.Errorf("unsupported shell %q: want bash or zsh", args[0])
	}
	fmt.Print(strings.ReplaceAll(script, "__PROXYCTL__", shellQuote(exe)))
	return nil
}

// shellQuote single-quotes s for bash/zsh.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// bashCompletion splits the line itself instead of using COMP_WORDS, which
// breaks "ip:port" apart at the colon (it's in COMP_WORDBREAKS).
const bashCompletion = `# bash completion for proxyctl
_proxyctl() {
    local line="${COMP_LINE:0:COMP_POINT}"
    local -a words
    read -ra words <<< "$line"
    [[ "$line" =~ [[:space:]]$ ]] && words+=("")
    local cur="${words[${#words[@]}-1]}"

    local IFS=$'\n'
    COMPREPLY=($(__PROXYCTL__ __complete "${words[@]:1}" 2>/dev/null))

    # bash only replaces the part of cur after the last colon, so strip
    # what's before it from each suggestion.
    if [[ "$cur" == *:* && "$COMP_WORDBREAKS" == *:* ]]; then
        local prefix="${cur%"${cur##*:}"}"
        local i
        for i in "${!COMPREPLY[@]}"; do
            COMPREPLY[$i]="${COMPREPLY[$i]#"$prefix"}"
        done
    fi
}
complete -F _proxyctl proxyctl
`

const zshCompletion = `# zsh completion for proxyctl
if (( ! $+functions[compdef] )); then
    autoload -Uz compinit && compinit
fi
_proxyctl() {
    local -a suggestions
    suggestions=(${(f)"$(__PROXYCTL__ __complete "${(@)words[2,CURRENT]}" 2>/dev/null)"})
    compadd -a suggestions
}
compdef _proxyctl proxyctl
`

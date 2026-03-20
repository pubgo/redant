package agentlinecmd

import (
	"strings"
	"sync"

	"github.com/pubgo/redant"
	agentlinemodule "github.com/pubgo/redant/pkg/agentline"
)

var registerRouteHookOnce sync.Once

func ensureRouteHookRegistered() {
	registerRouteHookOnce.Do(func() {
		redant.RegisterCommandDispatchHook(routeAgentCommandToAgentline)
	})
}

func routeAgentCommandToAgentline(input redant.CommandDispatchInput) (redant.CommandDispatchOutput, bool) {
	if input.Command == nil {
		return redant.CommandDispatchOutput{}, false
	}
	if !agentlinemodule.IsAgentEntryCommand(input.Command.Metadata) {
		return redant.CommandDispatchOutput{}, false
	}
	if input.Command.Name() == agentlinemodule.CommandName {
		return redant.CommandDispatchOutput{}, false
	}

	root := input.Parent
	if root == nil {
		root = input.Command
		for root.Parent() != nil {
			root = root.Parent()
		}
	}

	agentline := findCommandByNameOrAlias(root, agentlinemodule.CommandName)
	if agentline == nil || agentline == input.Command {
		return redant.CommandDispatchOutput{}, false
	}

	initialArgv := make([]string, 0, len(input.RawAllArgs)+4)
	if input.Consumed > 0 {
		initialArgv = append(initialArgv, input.RawAllArgs...)
	} else {
		initialArgv = append(initialArgv, commandPathFromAncestor(input.Parent, input.Command)...)
		initialArgv = append(initialArgv, input.RemainingArgs...)
	}
	if len(initialArgv) == 0 {
		initialArgv = append(initialArgv, commandPathFromAncestor(root, input.Command)...)
	}

	return redant.CommandDispatchOutput{
		Command: agentline,
		Args:    agentlinemodule.BuildBootstrapArgs(initialArgv),
	}, true
}

func findCommandByNameOrAlias(root *redant.Command, token string) *redant.Command {
	if root == nil {
		return nil
	}

	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}

	if root.Name() == token {
		return root
	}
	for _, alias := range root.Aliases {
		if strings.TrimSpace(alias) == token {
			return root
		}
	}

	for _, child := range root.Children {
		if found := findCommandByNameOrAlias(child, token); found != nil {
			return found
		}
	}

	return nil
}

func commandPathFromAncestor(ancestor, cmd *redant.Command) []string {
	if cmd == nil {
		return nil
	}
	if ancestor == cmd {
		return nil
	}

	parts := make([]string, 0, 4)
	for cur := cmd; cur != nil && cur != ancestor; cur = cur.Parent() {
		parts = append(parts, cur.Name())
	}

	if ancestor != nil {
		cur := cmd
		found := false
		for cur != nil {
			if cur == ancestor {
				found = true
				break
			}
			cur = cur.Parent()
		}
		if !found {
			return nil
		}
	}

	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}

	return parts
}

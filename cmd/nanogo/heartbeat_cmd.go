package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tvmaly/nanogo/core/heartbeat"
	fakesched "github.com/tvmaly/nanogo/core/scheduler/fake"
)

// heartbeatConfigPath returns the path to the persisted heartbeats file.
func heartbeatConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".nanogo", "heartbeats.json")
}

// loadHeartbeats reads persisted heartbeats from disk.
func loadHeartbeats() ([]heartbeat.Heartbeat, error) {
	path := heartbeatConfigPath()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var hbs []heartbeat.Heartbeat
	if err := json.Unmarshal(data, &hbs); err != nil {
		return nil, err
	}
	return hbs, nil
}

// saveHeartbeats writes heartbeats to disk.
func saveHeartbeats(hbs []heartbeat.Heartbeat) error {
	path := heartbeatConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(hbs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// runHeartbeatCmd dispatches heartbeat subcommands.
func runHeartbeatCmd(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: nanogo heartbeat <add|list|remove|test>")
	}
	switch args[0] {
	case "add":
		return heartbeatAdd(args[1:])
	case "list":
		return heartbeatList()
	case "remove":
		return heartbeatRemove(args[1:])
	case "test":
		return heartbeatTest(args[1:])
	default:
		return fmt.Errorf("unknown heartbeat subcommand %q", args[0])
	}
}

func heartbeatAdd(args []string) error {
	fs := flag.NewFlagSet("heartbeat add", flag.ContinueOnError)
	id := fs.String("id", "", "Heartbeat ID (required)")
	cron := fs.String("cron", "", "Cron spec or 'every Nm' interval (required)")
	action := fs.String("action", "", "Action kind: wake, skill, trigger, or tool")
	prompt := fs.String("prompt", "", "Prompt to send (wake action)")
	skill := fs.String("skill", "", "Skill to fire (skill action)")
	wake := fs.String("wake", "", "Prompt to send (wake action)")
	tool := fs.String("tool", "", "Tool to call (tool action)")
	when := fs.String("when", "", "Predicate name (trigger action)")
	triggerSkill := fs.String("trigger-skill", "", "Skill to fire when predicate is true (trigger action)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id == "" || *cron == "" {
		return fmt.Errorf("heartbeat add: --id and --cron are required")
	}
	hb := heartbeat.Heartbeat{ID: *id, Cron: *cron}
	switch {
	case *action == string(heartbeat.ActionWake):
		hb.Action = heartbeat.Action{Kind: heartbeat.ActionWake, Prompt: firstNonEmpty(*prompt, *wake)}
	case *action == string(heartbeat.ActionSkill):
		hb.Action = heartbeat.Action{Kind: heartbeat.ActionSkill, Skill: *skill}
	case *action == string(heartbeat.ActionTool):
		hb.Action = heartbeat.Action{Kind: heartbeat.ActionTool, Tool: *tool}
	case *action == string(heartbeat.ActionTrigger):
		hb.Action = heartbeat.Action{Kind: heartbeat.ActionTrigger, When: *when, Skill: *triggerSkill}
	case *skill != "":
		hb.Action = heartbeat.Action{Kind: heartbeat.ActionSkill, Skill: *skill}
	case *wake != "":
		hb.Action = heartbeat.Action{Kind: heartbeat.ActionWake, Prompt: *wake}
	case *tool != "":
		hb.Action = heartbeat.Action{Kind: heartbeat.ActionTool, Tool: *tool}
	case *when != "":
		hb.Action = heartbeat.Action{Kind: heartbeat.ActionTrigger, When: *when, Skill: *triggerSkill}
	default:
		return fmt.Errorf("heartbeat add: must specify one of --skill, --wake, --tool, or --when")
	}

	hbs, err := loadHeartbeats()
	if err != nil {
		return err
	}
	// Replace existing with same ID.
	updated := false
	for i, h := range hbs {
		if h.ID == *id {
			hbs[i] = hb
			updated = true
			break
		}
	}
	if !updated {
		hbs = append(hbs, hb)
	}
	if err := saveHeartbeats(hbs); err != nil {
		return err
	}
	fmt.Printf("heartbeat %q added (cron: %s)\n", *id, *cron)
	return nil
}

func heartbeatList() error {
	hbs, err := loadHeartbeats()
	if err != nil {
		return err
	}
	if len(hbs) == 0 {
		fmt.Println("(no heartbeats)")
		return nil
	}
	// Use a fake scheduler to get next fire time.
	sched := fakesched.New()
	for _, hb := range hbs {
		fmt.Printf("%-20s  %-20s  kind=%-10s", hb.ID, hb.Cron, hb.Action.Kind)
		switch hb.Action.Kind {
		case heartbeat.ActionSkill:
			fmt.Printf("  skill=%s", hb.Action.Skill)
		case heartbeat.ActionWake:
			fmt.Printf("  prompt=%q", hb.Action.Prompt)
		case heartbeat.ActionTool:
			fmt.Printf("  tool=%s", hb.Action.Tool)
		case heartbeat.ActionTrigger:
			fmt.Printf("  when=%s  skill=%s", hb.Action.When, hb.Action.Skill)
		}
		fmt.Println()
		_ = sched
	}
	return nil
}

func heartbeatRemove(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("heartbeat remove: ID required")
	}
	id := args[0]
	hbs, err := loadHeartbeats()
	if err != nil {
		return err
	}
	filtered := hbs[:0]
	found := false
	for _, hb := range hbs {
		if hb.ID == id {
			found = true
			continue
		}
		filtered = append(filtered, hb)
	}
	if !found {
		return fmt.Errorf("heartbeat %q not found", id)
	}
	if err := saveHeartbeats(filtered); err != nil {
		return err
	}
	fmt.Printf("heartbeat %q removed\n", id)
	return nil
}

func heartbeatTest(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("heartbeat test: ID required")
	}
	id := args[0]
	hbs, err := loadHeartbeats()
	if err != nil {
		return err
	}
	for _, hb := range hbs {
		if hb.ID == id {
			fmt.Printf("[dry-run] heartbeat %q would fire:\n", id)
			fmt.Printf("  kind: %s\n", hb.Action.Kind)
			switch hb.Action.Kind {
			case heartbeat.ActionSkill:
				fmt.Printf("  skill: %s\n", hb.Action.Skill)
			case heartbeat.ActionWake:
				fmt.Printf("  prompt: %s\n", hb.Action.Prompt)
			case heartbeat.ActionTool:
				fmt.Printf("  tool: %s, args: %v\n", hb.Action.Tool, hb.Action.Args)
			case heartbeat.ActionTrigger:
				fmt.Printf("  when: %s → skill: %s\n", hb.Action.When, hb.Action.Skill)
			}
			return nil
		}
	}
	return fmt.Errorf("heartbeat %q not found", id)
}

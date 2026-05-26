package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	helpfiles "github.com/tvmaly/nanogo/ext/help/files"
	"github.com/tvmaly/nanogo/modules/help"
)

func runHelpCmd(args []string) error {
	fs := flag.NewFlagSet("help", flag.ContinueOnError)
	root := fs.String("root", defaultHelpRoot(), "Help pack root")
	if err := fs.Parse(args); err != nil {
		return err
	}
	svc, roots, err := buildHelpService(*root)
	if err != nil {
		return err
	}
	rest := fs.Args()
	ctx := context.Background()
	if len(rest) == 0 {
		fmt.Println("Nanogo help topics:")
		for _, id := range roots {
			resp, err := svc.Topic(ctx, help.TopicRequest{ID: id})
			if err != nil {
				return err
			}
			fmt.Printf("- %s: %s\n", resp.Topic.ID, resp.Topic.Summary)
		}
		return nil
	}
	if rest[0] == "search" {
		if len(rest) < 2 {
			return fmt.Errorf("usage: nanogo help search <query>")
		}
		resp, err := svc.Search(ctx, help.SearchRequest{Query: strings.Join(rest[1:], " "), Limit: 10, IncludeBody: true})
		if err != nil {
			return err
		}
		for _, hit := range resp.Hits {
			fmt.Printf("%s\t%s\t%s\n", hit.ID, hit.Title, hit.Snippet)
		}
		return nil
	}
	if rest[0] == "validate" {
		resp, err := svc.Validate(ctx, help.ValidateRequest{})
		if err != nil {
			var ve help.ValidationError
			if errors.As(err, &ve) {
				for _, msg := range ve.Errors {
					fmt.Fprintln(os.Stderr, msg)
				}
				return err
			}
			return err
		}
		if !resp.OK {
			for _, msg := range resp.Errors {
				fmt.Fprintln(os.Stderr, msg)
			}
			return fmt.Errorf("help validation failed")
		}
		fmt.Println("help validation: ok")
		return nil
	}
	resp, err := svc.Render(ctx, help.RenderRequest{TopicID: rest[0], Format: help.FormatMarkdown, Width: 100})
	if err != nil {
		return err
	}
	fmt.Print(resp.Text)
	return nil
}

func buildHelpService(root string) (*help.LocalService, []string, error) {
	pack, err := helpfiles.New(root).Load(context.Background())
	if err != nil {
		return nil, nil, err
	}
	cat, err := help.NewCatalog(pack)
	if err != nil {
		return nil, nil, err
	}
	return help.NewService(cat), cat.RootTopics(), nil
}

func defaultHelpRoot() string {
	dir, err := os.Getwd()
	if err == nil {
		for {
			candidate := filepath.Join(dir, "docs", "help")
			if _, err := os.Stat(filepath.Join(candidate, "index.yaml")); err == nil {
				return candidate
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return filepath.Join(defaultWorkspaceDir(), "docs", "help")
}

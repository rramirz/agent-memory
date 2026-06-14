package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/rramirz/agent-memory/internal/config"
	"github.com/rramirz/agent-memory/internal/models"
	"github.com/rramirz/agent-memory/internal/outbox"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func main() {
	root := &cobra.Command{
		Use:   "memory",
		Short: "Agent memory CLI",
	}
	root.AddCommand(
		newInitCmd(),
		newAddCmd(),
		newSearchCmd(),
		newSyncCmd(),
		newFlushCmd(),
		newStatusCmd(),
	)
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func newInitCmd() *cobra.Command {
	var org, project, repo string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize repo config and docs/ai/",
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := os.Stat(".agent-memory.yaml"); os.IsNotExist(err) {
				repoCfg := config.RepoConfig{
					Org:     org,
					Project: project,
					Repo:    repo,
					Sync: config.SyncConfig{
						OutputDir: "docs/ai",
						Files:     []string{"current-state.md", "decisions.md", "architecture.md", "known-issues.md"},
					},
				}
				data, err := yaml.Marshal(repoCfg)
				if err != nil {
					return fmt.Errorf("marshal repo config: %w", err)
				}
				if err := os.WriteFile(".agent-memory.yaml", data, 0644); err != nil {
					return fmt.Errorf("write .agent-memory.yaml: %w", err)
				}
				fmt.Println("created .agent-memory.yaml")
			} else {
				fmt.Println(".agent-memory.yaml already exists, skipping")
			}

			if err := os.MkdirAll("docs/ai", 0755); err != nil {
				return fmt.Errorf("create docs/ai: %w", err)
			}
			fmt.Println("created docs/ai/")

			home, _ := os.UserHomeDir()
			wsPath := filepath.Join(home, ".agent-memory", "config.yaml")
			if _, err := os.Stat(wsPath); os.IsNotExist(err) {
				fmt.Printf("\nworkstation config missing: %s\n", wsPath)
				fmt.Printf("create it:\n\n")
				fmt.Printf("  workstation: home-mac\n")
				fmt.Printf("  default_org: personal\n")
				fmt.Printf("  allowed_orgs:\n")
				fmt.Printf("    - personal\n")
				fmt.Printf("  api_url: https://memory.theramirez.casa\n")
				fmt.Printf("  token_env: MEMORY_TOKEN\n")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&org, "org", "", "org (personal|arrive|logicbroker)")
	cmd.Flags().StringVar(&project, "project", "", "project name")
	cmd.Flags().StringVar(&repo, "repo", "", "repo name")
	_ = cmd.MarkFlagRequired("org")
	return cmd
}

func newAddCmd() *cobra.Command {
	var memType, title, body, scope string
	var tags []string
	var importance int
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a memory",
		RunE: func(cmd *cobra.Command, args []string) error {
			wsCfg, repoCfg, token, apiURL, err := loadContext()
			if err != nil {
				return err
			}
			req := models.CreateMemoryRequest{
				Org:         repoCfg.Org,
				Project:     repoCfg.Project,
				Repo:        repoCfg.Repo,
				Workstation: wsCfg.Workstation,
				Scope:       scope,
				Type:        memType,
				Title:       title,
				Body:        body,
				Tags:        tags,
				Importance:  importance,
				Source:      models.SourceManual,
			}
			if err := postMemory(apiURL, token, req); err != nil {
				fmt.Fprintf(os.Stderr, "API unavailable (%v), queuing\n", err)
				if err2 := outbox.Write(req); err2 != nil {
					return fmt.Errorf("write outbox: %w", err2)
				}
				fmt.Println("queued — run `memory flush` when online")
				return nil
			}
			fmt.Println("memory saved")
			return nil
		},
	}
	cmd.Flags().StringVar(&memType, "type", "note", "type (decision|session_summary|architecture|runbook|known_issue|task|preference|note|idea|skill|agent|prompt_pattern)")
	cmd.Flags().StringVar(&title, "title", "", "title")
	cmd.Flags().StringVar(&body, "body", "", "body")
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "tag (repeatable)")
	cmd.Flags().IntVar(&importance, "importance", 5, "importance 1-10")
	cmd.Flags().StringVar(&scope, "scope", "repo", "scope")
	_ = cmd.MarkFlagRequired("title")
	_ = cmd.MarkFlagRequired("body")
	return cmd
}

func newSearchCmd() *cobra.Command {
	var memType, project, repo string
	var limit int
	cmd := &cobra.Command{
		Use:   "search [query]",
		Short: "Search memories",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, repoCfg, token, apiURL, err := loadContext()
			if err != nil {
				return err
			}
			params := url.Values{}
			params.Set("org", repoCfg.Org)
			params.Set("q", args[0])
			params.Set("limit", strconv.Itoa(limit))
			if memType != "" {
				params.Set("type", memType)
			}
			if p := firstNonEmpty(project, repoCfg.Project); p != "" {
				params.Set("project", p)
			}
			if r := firstNonEmpty(repo, repoCfg.Repo); r != "" {
				params.Set("repo", r)
			}

			req, _ := http.NewRequest(http.MethodGet, apiURL+"/v1/memories/search?"+params.Encode(), nil)
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
			if err != nil {
				return fmt.Errorf("search: %w", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("HTTP %d", resp.StatusCode)
			}

			var result models.SearchMemoriesResponse
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return fmt.Errorf("decode: %w", err)
			}
			if len(result.Memories) == 0 {
				fmt.Println("no results")
				return nil
			}

			tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "TYPE\tIMPORTANCE\tTITLE\tTAGS")
			for _, m := range result.Memories {
				fmt.Fprintf(tw, "%s\t%d\t%s\t%s\n", m.Type, m.Importance, m.Title, strings.Join(m.Tags, ","))
			}
			tw.Flush()
			fmt.Printf("\n%d result(s)\n", result.Total)
			return nil
		},
	}
	cmd.Flags().StringVar(&memType, "type", "", "filter by type")
	cmd.Flags().StringVar(&project, "project", "", "override project")
	cmd.Flags().StringVar(&repo, "repo", "", "override repo")
	cmd.Flags().IntVar(&limit, "limit", 20, "max results")
	return cmd
}

func newSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Pull context from API and write docs/ai/",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, repoCfg, token, apiURL, err := loadContext()
			if err != nil {
				return err
			}
			params := url.Values{}
			params.Set("org", repoCfg.Org)
			if repoCfg.Project != "" {
				params.Set("project", repoCfg.Project)
			}
			if repoCfg.Repo != "" {
				params.Set("repo", repoCfg.Repo)
			}

			req, _ := http.NewRequest(http.MethodGet, apiURL+"/v1/context?"+params.Encode(), nil)
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
			if err != nil {
				return fmt.Errorf("sync: %w", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("HTTP %d", resp.StatusCode)
			}

			var result models.ContextResponse
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return fmt.Errorf("decode: %w", err)
			}
			for _, f := range result.Files {
				if err := os.MkdirAll(filepath.Dir(f.Path), 0755); err != nil {
					return fmt.Errorf("mkdir %s: %w", f.Path, err)
				}
				if err := os.WriteFile(f.Path, []byte(f.Content), 0644); err != nil {
					return fmt.Errorf("write %s: %w", f.Path, err)
				}
				fmt.Println("updated", f.Path)
			}
			fmt.Printf("\n%d file(s) synced\n", len(result.Files))
			return nil
		},
	}
}

func newFlushCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "flush",
		Short: "Send queued outbox entries to the API",
		RunE: func(cmd *cobra.Command, args []string) error {
			wsCfg, token, apiURL, err := loadWorkstationCtx()
			if err != nil {
				return err
			}
			entries, err := outbox.ReadAll()
			if err != nil {
				return fmt.Errorf("read outbox: %w", err)
			}
			if len(entries) == 0 {
				fmt.Println("outbox empty")
				return nil
			}
			sent, failed := 0, 0
			for _, e := range entries {
				var req models.CreateMemoryRequest
				if err := json.Unmarshal(e.Data, &req); err != nil {
					fmt.Fprintf(os.Stderr, "skip %s: parse error\n", e.Filename)
					failed++
					continue
				}
				if !wsCfg.CanAccessOrg(req.Org) {
					fmt.Fprintf(os.Stderr, "skip %s: org %q not allowed\n", e.Filename, req.Org)
					failed++
					continue
				}
				if err := postMemory(apiURL, token, req); err != nil {
					fmt.Fprintf(os.Stderr, "skip %s: %v\n", e.Filename, err)
					failed++
					continue
				}
				_ = outbox.Delete(e.Path)
				fmt.Println("flushed", e.Filename)
				sent++
			}
			fmt.Printf("\nsent: %d  failed: %d\n", sent, failed)
			return nil
		},
	}
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show memory service status",
		RunE: func(cmd *cobra.Command, args []string) error {
			wsCfg, _, apiURL, err := loadWorkstationCtx()
			if err != nil {
				return err
			}
			repoCfg, _ := config.LoadRepoConfig()

			apiStatus := "reachable"
			resp, err := (&http.Client{Timeout: 3 * time.Second}).Get(apiURL + "/v1/healthz")
			if err != nil || resp.StatusCode != http.StatusOK {
				apiStatus = "unreachable"
			}
			if resp != nil {
				resp.Body.Close()
			}

			count, _ := outbox.Count()

			tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(tw, "workstation:\t%s\n", wsCfg.Workstation)
			if repoCfg != nil {
				fmt.Fprintf(tw, "org:\t%s\n", repoCfg.Org)
				fmt.Fprintf(tw, "project:\t%s\n", repoCfg.Project)
				fmt.Fprintf(tw, "repo:\t%s\n", repoCfg.Repo)
			}
			fmt.Fprintf(tw, "api:\t%s\n", apiStatus)
			fmt.Fprintf(tw, "outbox:\t%d pending\n", count)
			return tw.Flush()
		},
	}
}

func loadWorkstationCtx() (*config.WorkstationConfig, string, string, error) {
	wsCfg, err := config.LoadWorkstationConfig()
	if err != nil {
		return nil, "", "", fmt.Errorf("workstation config: %w", err)
	}
	if wsCfg.APIURL == "" {
		return nil, "", "", fmt.Errorf("api_url not set in ~/.agent-memory/config.yaml")
	}
	token := wsCfg.Token()
	if token == "" {
		env := wsCfg.TokenEnv
		if env == "" {
			env = "MEMORY_TOKEN"
		}
		return nil, "", "", fmt.Errorf("%s env var not set", env)
	}
	return wsCfg, token, wsCfg.APIURL, nil
}

func loadContext() (*config.WorkstationConfig, *config.RepoConfig, string, string, error) {
	wsCfg, token, apiURL, err := loadWorkstationCtx()
	if err != nil {
		return nil, nil, "", "", err
	}
	repoCfg, err := config.LoadRepoConfig()
	if err != nil {
		return nil, nil, "", "", err
	}
	if !wsCfg.CanAccessOrg(repoCfg.Org) {
		return nil, nil, "", "", fmt.Errorf("workstation not allowed to access org %q", repoCfg.Org)
	}
	return wsCfg, repoCfg, token, apiURL, nil
}

func postMemory(apiURL, token string, req models.CreateMemoryRequest) error {
	b, _ := json.Marshal(req)
	httpReq, _ := http.NewRequest(http.MethodPost, apiURL+"/v1/memories", bytes.NewReader(b))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)
	resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

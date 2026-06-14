package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
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
		newImportCmd(),
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

func newImportCmd() *cobra.Command {
	var dir string
	var files []string
	var org, scope, defaultType string
	var dryRun bool
	var skipSections []string
	var minBody int

	cmd := &cobra.Command{
		Use:   "import",
		Short: "Bulk-import local markdown memory files (dreams/notes) into the service",
		Long: "Parses OpenCode memory markdown (notes.md, dreams/core.md, dreams/local.md)\n" +
			"into individual memories and POSTs them. Records are tagged with this\n" +
			"workstation; org defaults to the workstation's default_org. Use --dry-run\n" +
			"to preview without writing.",
		RunE: func(cmd *cobra.Command, args []string) error {
			wsCfg, token, apiURL, err := loadWorkstationCtx()
			if err != nil {
				return err
			}
			if org == "" {
				org = wsCfg.DefaultOrg
			}
			if org == "" {
				return fmt.Errorf("no org: pass --org or set default_org in workstation config")
			}
			if !models.ValidOrgs[org] {
				return fmt.Errorf("invalid org %q (personal|arrive|logicbroker)", org)
			}
			if !wsCfg.CanAccessOrg(org) {
				return fmt.Errorf("workstation not allowed to access org %q", org)
			}

			targets := files
			if len(targets) == 0 {
				if dir == "" {
					home, _ := os.UserHomeDir()
					dir = filepath.Join(home, ".config", "opencode", "context")
				}
				for _, rel := range []string{"notes.md", "dreams/core.md", "dreams/local.md"} {
					p := filepath.Join(dir, rel)
					if _, statErr := os.Stat(p); statErr == nil {
						targets = append(targets, p)
					}
				}
			}
			if len(targets) == 0 {
				return fmt.Errorf("no input files found (looked in %s)", dir)
			}

			skip := map[string]bool{}
			for _, s := range skipSections {
				skip[strings.ToLower(strings.TrimSpace(s))] = true
			}

			var reqs []models.CreateMemoryRequest
			for _, f := range targets {
				entries, perr := parseMemoryFile(f, skip, minBody)
				if perr != nil {
					fmt.Fprintf(os.Stderr, "skip %s: %v\n", f, perr)
					continue
				}
				stem := strings.TrimSuffix(filepath.Base(f), ".md")
				for _, e := range entries {
					typ := classifyType(e.section, e.title, e.body, defaultType)
					reqs = append(reqs, models.CreateMemoryRequest{
						Org:         org,
						Workstation: wsCfg.Workstation,
						Scope:       scope,
						Type:        typ,
						Title:       e.title,
						Body:        e.body,
						Tags:        dedupeTags("imported", wsCfg.Workstation, stem, slug(e.section)),
						Importance:  importanceFor(typ),
						Source:      models.SourceImport,
					})
				}
			}

			if len(reqs) == 0 {
				fmt.Println("nothing to import")
				return nil
			}

			if dryRun {
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "#\tTYPE\tIMP\tORG\tWORKSTATION\tTITLE")
				for i, r := range reqs {
					fmt.Fprintf(tw, "%d\t%s\t%d\t%s\t%s\t%s\n", i+1, r.Type, r.Importance, r.Org, r.Workstation, truncate(r.Title, 64))
				}
				tw.Flush()
				fmt.Printf("\n[dry-run] %d memories would be imported from %d file(s)\n", len(reqs), len(targets))
				return nil
			}

			created, queued, failed := 0, 0, 0
			for _, r := range reqs {
				if perr := postMemory(apiURL, token, r); perr != nil {
					if werr := outbox.Write(r); werr != nil {
						fmt.Fprintf(os.Stderr, "FAILED %q: post %v; outbox %v\n", r.Title, perr, werr)
						failed++
						continue
					}
					queued++
					continue
				}
				created++
			}
			fmt.Printf("imported: %d created, %d queued, %d failed (org=%s, workstation=%s, files=%d)\n",
				created, queued, failed, org, wsCfg.Workstation, len(targets))
			if queued > 0 {
				fmt.Println("run `memory flush` to send queued entries when the API is reachable")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "base dir to scan (default ~/.config/opencode/context)")
	cmd.Flags().StringArrayVar(&files, "file", nil, "explicit markdown file(s) to import (repeatable; overrides --dir)")
	cmd.Flags().StringVar(&org, "org", "", "org (default: workstation default_org)")
	cmd.Flags().StringVar(&scope, "scope", models.ScopeGlobal, "scope")
	cmd.Flags().StringVar(&defaultType, "default-type", models.TypeNote, "fallback type when the heuristic finds none")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "parse and print, do not write")
	cmd.Flags().StringArrayVar(&skipSections, "skip", []string{"Mined Session IDs"}, "section titles to skip (repeatable)")
	cmd.Flags().IntVar(&minBody, "min-body", 1, "skip entries whose trimmed body is shorter than N chars")
	return cmd
}

type memEntry struct {
	section string
	title   string
	body    string
}

var (
	reH2       = regexp.MustCompile(`^##\s+(.+?)\s*$`)
	reH3       = regexp.MustCompile(`^###\s+(.+?)\s*$`)
	reTopBull  = regexp.MustCompile(`^-\s+(.*)$`)
	reBoldLead = regexp.MustCompile("^`?\\*\\*(.+?)\\*\\*")
	reDate     = regexp.MustCompile(`\s*_*\(*\d{4}-\d{2}-\d{2}\)*_*`)
	reSlug     = regexp.MustCompile(`[^a-z0-9]+`)
)

func parseMemoryFile(path string, skip map[string]bool, minBody int) ([]memEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")

	type section struct {
		title string
		body  []string
	}
	var sections []section
	cur := section{}
	for _, ln := range lines {
		if m := reH2.FindStringSubmatch(ln); m != nil {
			sections = append(sections, cur)
			cur = section{title: m[1]}
			continue
		}
		cur.body = append(cur.body, ln)
	}
	sections = append(sections, cur)

	var out []memEntry
	for _, s := range sections {
		title := cleanTitle(s.title)
		if title == "" {
			continue // preamble before first H2 (file header/comments)
		}
		if skip[strings.ToLower(title)] {
			continue
		}
		body := strings.Join(s.body, "\n")
		switch {
		case containsLineMatch(s.body, reH3):
			out = append(out, splitByH3(title, s.body, minBody)...)
		case hasBoldLedBullet(s.body):
			out = append(out, splitByBullet(title, s.body, minBody)...)
		default:
			if b := strings.TrimSpace(body); meaningfulLen(b) >= minBody {
				out = append(out, memEntry{section: title, title: title, body: b})
			}
		}
	}
	return out, nil
}

func splitByH3(section string, body []string, minBody int) []memEntry {
	var out []memEntry
	var curTitle string
	var curBody, preamble []string
	seen := false
	flush := func() {
		b := strings.TrimSpace(strings.Join(curBody, "\n"))
		if curTitle != "" && meaningfulLen(b) >= minBody {
			out = append(out, memEntry{section: section, title: cleanTitle(curTitle), body: b})
		}
		curBody = nil
	}
	for _, ln := range body {
		if m := reH3.FindStringSubmatch(ln); m != nil {
			if !seen {
				if pre := strings.TrimSpace(strings.Join(preamble, "\n")); meaningfulLen(pre) >= minBody {
					out = append(out, memEntry{section: section, title: section, body: pre})
				}
				seen = true
			} else {
				flush()
			}
			curTitle = m[1]
			continue
		}
		if seen {
			curBody = append(curBody, ln)
		} else {
			preamble = append(preamble, ln)
		}
	}
	if seen {
		flush()
	}
	return out
}

func splitByBullet(section string, body []string, minBody int) []memEntry {
	var out []memEntry
	var cur, preamble []string
	started := false
	flush := func() {
		if len(cur) == 0 {
			return
		}
		b := strings.TrimSpace(strings.Join(cur, "\n"))
		title := bulletTitle(cur[0])
		if title == "" {
			title = cleanTitle(truncate(b, 60))
		}
		if title == "" {
			title = section
		}
		if meaningfulLen(b) >= minBody {
			out = append(out, memEntry{section: section, title: title, body: b})
		}
		cur = nil
	}
	for _, ln := range body {
		if reTopBull.MatchString(ln) {
			if !started {
				if pre := strings.TrimSpace(strings.Join(preamble, "\n")); meaningfulLen(pre) >= minBody {
					out = append(out, memEntry{section: section, title: section, body: pre})
				}
				started = true
			} else {
				flush()
			}
			cur = []string{ln}
			continue
		}
		if started {
			cur = append(cur, ln)
		} else {
			preamble = append(preamble, ln)
		}
	}
	flush()
	return out
}

func bulletTitle(line string) string {
	content := line
	if m := reTopBull.FindStringSubmatch(line); m != nil {
		content = m[1]
	}
	content = strings.TrimSpace(content)
	if bm := reBoldLead.FindStringSubmatch(content); bm != nil {
		if t := cleanTitle(bm[1]); t != "" {
			return t
		}
	}
	c := strings.ReplaceAll(strings.ReplaceAll(content, "`", ""), "*", "")
	if i := strings.IndexAny(c, ":."); i > 0 && i < 70 {
		c = c[:i]
	}
	return cleanTitle(truncate(c, 70))
}

func cleanTitle(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "`", "")
	s = reDate.ReplaceAllString(s, "")
	s = strings.Trim(strings.TrimSpace(s), "—-:* ")
	return strings.TrimSpace(s)
}

func meaningfulLen(s string) int {
	r := strings.NewReplacer("-", "", "*", "", "`", "", "#", "", ">", "", "|", "")
	return len(strings.TrimSpace(r.Replace(s)))
}

func containsLineMatch(lines []string, re *regexp.Regexp) bool {
	for _, ln := range lines {
		if re.MatchString(ln) {
			return true
		}
	}
	return false
}

func hasBoldLedBullet(body []string) bool {
	for _, ln := range body {
		if m := reTopBull.FindStringSubmatch(ln); m != nil {
			if reBoldLead.MatchString(strings.TrimSpace(m[1])) {
				return true
			}
		}
	}
	return false
}

func classifyType(section, title, body, def string) string {
	h := strings.ToLower(section + " | " + title)
	switch {
	case containsAny(h, "pitfall", "gotcha", "known issue", "known-issue", "bug", "fails", "broken"):
		return models.TypeKnownIssue
	case containsAny(h, "preference", "prefer", "working preference"):
		return models.TypePreference
	case containsAny(h, "decision", "decided", "pivot", "non-goal"):
		return models.TypeDecision
	case containsAny(h, "architecture", "sso gate", "fleet", "topology", "data model", "design"):
		return models.TypeArchitecture
	case containsAny(h, "pattern", "runbook", "setup", "onboard", "deploy", "workflow", "install", "provider strategy"):
		return models.TypeRunbook
	case containsAny(h, "skill"):
		return models.TypeSkill
	}
	if def == "" {
		return models.TypeNote
	}
	return def
}

func importanceFor(t string) int {
	switch t {
	case models.TypePreference:
		return 8
	case models.TypeDecision, models.TypeKnownIssue:
		return 7
	case models.TypeArchitecture, models.TypeRunbook:
		return 6
	default:
		return 5
	}
}

func containsAny(s string, subs ...string) bool {
	for _, x := range subs {
		if strings.Contains(s, x) {
			return true
		}
	}
	return false
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = reSlug.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

func dedupeTags(in ...string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range in {
		t = strings.TrimSpace(t)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

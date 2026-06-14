package contextgen

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rramirz/agent-memory/internal/db"
	"github.com/rramirz/agent-memory/internal/models"
)

type Generator struct {
	db *db.DB
}

func New(database *db.DB) *Generator {
	return &Generator{db: database}
}

type fileSpec struct {
	path  string
	title string
	types []string
	limit int
}

var contextFiles = []fileSpec{
	{
		path:  "docs/ai/current-state.md",
		title: "Current State",
		types: []string{models.TypeNote, models.TypeTask, models.TypeSessionSummary, models.TypePreference},
		limit: 15,
	},
	{
		path:  "docs/ai/decisions.md",
		title: "Decisions",
		types: []string{models.TypeDecision},
		limit: 50,
	},
	{
		path:  "docs/ai/architecture.md",
		title: "Architecture",
		types: []string{models.TypeArchitecture, models.TypeRunbook},
		limit: 30,
	},
	{
		path:  "docs/ai/known-issues.md",
		title: "Known Issues",
		types: []string{models.TypeKnownIssue},
		limit: 30,
	},
	{
		path:  "docs/ai/ideas.md",
		title: "Ideas & Building Blocks",
		types: []string{models.TypeIdea, models.TypeSkill, models.TypeAgent, models.TypePromptPattern},
		limit: 50,
	},
}

func (g *Generator) Generate(ctx context.Context, org, project, repo string) ([]models.ContextFile, error) {
	var result []models.ContextFile
	for _, spec := range contextFiles {
		var memories []models.Memory
		for _, t := range spec.types {
			mems, err := g.db.GetMemoriesByType(ctx, org, project, repo, t, spec.limit)
			if err != nil {
				return nil, fmt.Errorf("generate %s: %w", spec.path, err)
			}
			memories = append(memories, mems...)
		}
		result = append(result, models.ContextFile{
			Path:    spec.path,
			Content: renderMarkdown(spec.title, org, project, repo, memories),
		})
	}
	return result, nil
}

func renderMarkdown(title, org, project, repo string, memories []models.Memory) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# %s\n\n", title))
	sb.WriteString(fmt.Sprintf("*Generated: %s*\n", time.Now().UTC().Format("2006-01-02")))

	meta := fmt.Sprintf("*Org: %s", org)
	if project != "" {
		meta += fmt.Sprintf(" | Project: %s", project)
	}
	if repo != "" {
		meta += fmt.Sprintf(" | Repo: %s", repo)
	}
	sb.WriteString(meta + "*\n\n")

	if len(memories) == 0 {
		sb.WriteString("*No entries.*\n")
		return sb.String()
	}

	sb.WriteString("---\n\n")
	for _, m := range memories {
		sb.WriteString(fmt.Sprintf("## [%s] %s (importance: %d)\n", m.Type, m.Title, m.Importance))
		sb.WriteString(fmt.Sprintf("*%s*\n\n", m.UpdatedAt.Format("2006-01-02")))
		sb.WriteString(m.Body + "\n")
		if len(m.Tags) > 0 {
			sb.WriteString(fmt.Sprintf("\nTags: %s\n", strings.Join(m.Tags, ", ")))
		}
		sb.WriteString("\n---\n\n")
	}
	return sb.String()
}

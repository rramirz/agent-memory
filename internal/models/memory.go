package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	StatusActive     = "active"
	StatusSuperseded = "superseded"
	StatusArchived   = "archived"
	StatusDeleted    = "deleted"

	ScopeGlobal  = "global"
	ScopeOrg     = "org"
	ScopeProject = "project"
	ScopeRepo    = "repo"
	ScopeSession = "session"

	TypeDecision       = "decision"
	TypeSessionSummary = "session_summary"
	TypeArchitecture   = "architecture"
	TypeRunbook        = "runbook"
	TypeKnownIssue     = "known_issue"
	TypeTask           = "task"
	TypePreference     = "preference"
	TypeNote           = "note"
	TypeIdea           = "idea"
	TypeSkill          = "skill"
	TypeAgent          = "agent"
	TypePromptPattern  = "prompt_pattern"

	SourceManual  = "manual"
	SourceSession = "session"
	SourceSync    = "sync"
	SourceImport  = "import"
	SourcePlugin  = "plugin"
)

var ValidOrgs = map[string]bool{
	"arrive":      true,
	"logicbroker": true,
	"personal":    true,
}

type Memory struct {
	ID          primitive.ObjectID `bson:"_id,omitempty"       json:"id,omitempty"`
	Org         string             `bson:"org"                 json:"org"`
	Project     string             `bson:"project,omitempty"   json:"project,omitempty"`
	Repo        string             `bson:"repo,omitempty"      json:"repo,omitempty"`
	Workstation string             `bson:"workstation,omitempty" json:"workstation,omitempty"`
	Scope       string             `bson:"scope,omitempty"     json:"scope,omitempty"`
	Type        string             `bson:"type"                json:"type"`
	Title       string             `bson:"title"               json:"title"`
	Body        string             `bson:"body"                json:"body"`
	Tags        []string           `bson:"tags,omitempty"      json:"tags,omitempty"`
	Importance  int                `bson:"importance"          json:"importance"`
	Status      string             `bson:"status"              json:"status"`
	Source      string             `bson:"source,omitempty"    json:"source,omitempty"`
	CreatedAt   time.Time          `bson:"created_at"          json:"created_at"`
	UpdatedAt   time.Time          `bson:"updated_at"          json:"updated_at"`
}

type CreateMemoryRequest struct {
	Org         string   `json:"org"`
	Project     string   `json:"project,omitempty"`
	Repo        string   `json:"repo,omitempty"`
	Workstation string   `json:"workstation,omitempty"`
	Scope       string   `json:"scope,omitempty"`
	Type        string   `json:"type"`
	Title       string   `json:"title"`
	Body        string   `json:"body"`
	Tags        []string `json:"tags,omitempty"`
	Importance  int      `json:"importance,omitempty"`
	Source      string   `json:"source,omitempty"`
}

type CreateMemoryResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type SearchMemoriesResponse struct {
	Memories []Memory `json:"memories"`
	Total    int      `json:"total"`
}

type ContextFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type ContextResponse struct {
	Org     string        `json:"org"`
	Project string        `json:"project,omitempty"`
	Repo    string        `json:"repo,omitempty"`
	Files   []ContextFile `json:"files"`
}

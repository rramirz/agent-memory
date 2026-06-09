package db

import (
	"context"
	"fmt"
	"time"

	"github.com/rramirz/agent-memory/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type DB struct {
	client   *mongo.Client
	database string
}

func Connect(ctx context.Context, uri, database string) (*DB, error) {
	opts := options.Client().ApplyURI(uri)
	client, err := mongo.Connect(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("connect mongodb: %w", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		return nil, fmt.Errorf("ping mongodb: %w", err)
	}
	return &DB{client: client, database: database}, nil
}

func (d *DB) Close(ctx context.Context) error {
	return d.client.Disconnect(ctx)
}

func (d *DB) col() *mongo.Collection {
	return d.client.Database(d.database).Collection("memories")
}

func (d *DB) EnsureIndexes(ctx context.Context) error {
	coll := d.col()
	compound := []mongo.IndexModel{
		{Keys: bson.D{{Key: "org", Value: 1}, {Key: "project", Value: 1}, {Key: "repo", Value: 1}, {Key: "updated_at", Value: -1}}},
		{Keys: bson.D{{Key: "org", Value: 1}, {Key: "tags", Value: 1}}},
		{Keys: bson.D{{Key: "org", Value: 1}, {Key: "type", Value: 1}}},
		{Keys: bson.D{{Key: "org", Value: 1}, {Key: "status", Value: 1}}},
	}
	if _, err := coll.Indexes().CreateMany(ctx, compound); err != nil {
		return fmt.Errorf("create compound indexes: %w", err)
	}
	textIdx := mongo.IndexModel{
		Keys: bson.D{
			{Key: "title", Value: "text"},
			{Key: "body", Value: "text"},
			{Key: "tags", Value: "text"},
		},
	}
	if _, err := coll.Indexes().CreateOne(ctx, textIdx); err != nil {
		return fmt.Errorf("create text index: %w", err)
	}
	return nil
}

func (d *DB) CreateMemory(ctx context.Context, m *models.Memory) error {
	m.ID = primitive.NewObjectID()
	now := time.Now().UTC()
	m.CreatedAt = now
	m.UpdatedAt = now
	if _, err := d.col().InsertOne(ctx, m); err != nil {
		return fmt.Errorf("insert memory: %w", err)
	}
	return nil
}

type SearchParams struct {
	Org     string
	Query   string
	Project string
	Repo    string
	Type    string
	Tag     string
	Limit   int
}

func (d *DB) SearchMemories(ctx context.Context, p SearchParams) ([]models.Memory, error) {
	if p.Limit <= 0 {
		p.Limit = 20
	}
	filter := bson.D{
		{Key: "org", Value: p.Org},
		{Key: "status", Value: models.StatusActive},
	}
	if p.Query != "" {
		filter = append(filter, bson.E{Key: "$text", Value: bson.D{{Key: "$search", Value: p.Query}}})
	}
	if p.Project != "" {
		filter = append(filter, bson.E{Key: "project", Value: p.Project})
	}
	if p.Repo != "" {
		filter = append(filter, bson.E{Key: "repo", Value: p.Repo})
	}
	if p.Type != "" {
		filter = append(filter, bson.E{Key: "type", Value: p.Type})
	}
	if p.Tag != "" {
		filter = append(filter, bson.E{Key: "tags", Value: p.Tag})
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "importance", Value: -1}, {Key: "updated_at", Value: -1}}).
		SetLimit(int64(p.Limit))

	cur, err := d.col().Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("search memories: %w", err)
	}
	defer cur.Close(ctx)

	var results []models.Memory
	if err := cur.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("decode memories: %w", err)
	}
	return results, nil
}

func (d *DB) GetMemoriesByType(ctx context.Context, org, project, repo, memType string, limit int) ([]models.Memory, error) {
	filter := bson.D{
		{Key: "org", Value: org},
		{Key: "status", Value: models.StatusActive},
		{Key: "type", Value: memType},
	}
	if project != "" {
		filter = append(filter, bson.E{Key: "project", Value: project})
	}
	if repo != "" {
		filter = append(filter, bson.E{Key: "repo", Value: repo})
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "importance", Value: -1}, {Key: "updated_at", Value: -1}}).
		SetLimit(int64(limit))

	cur, err := d.col().Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("get memories type=%s: %w", memType, err)
	}
	defer cur.Close(ctx)

	var results []models.Memory
	if err := cur.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("decode memories: %w", err)
	}
	return results, nil
}

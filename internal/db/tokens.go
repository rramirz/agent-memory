package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rramirz/agent-memory/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (d *DB) tokensCol() *mongo.Collection {
	return d.client.Database(d.database).Collection("tokens")
}

func (d *DB) CreateToken(ctx context.Context, t *models.Token) error {
	t.ID = primitive.NewObjectID()
	t.CreatedAt = time.Now().UTC()
	if _, err := d.tokensCol().InsertOne(ctx, t); err != nil {
		return fmt.Errorf("insert token: %w", err)
	}
	return nil
}

func (d *DB) ListTokens(ctx context.Context) ([]models.Token, error) {
	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}})
	cur, err := d.tokensCol().Find(ctx, bson.D{}, opts)
	if err != nil {
		return nil, fmt.Errorf("list tokens: %w", err)
	}
	defer cur.Close(ctx)

	var results []models.Token
	if err := cur.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("decode tokens: %w", err)
	}
	return results, nil
}

func (d *DB) RevokeToken(ctx context.Context, id primitive.ObjectID) (bool, error) {
	filter := bson.D{
		{Key: "_id", Value: id},
		{Key: "revoked_at", Value: nil},
	}
	update := bson.D{{Key: "$set", Value: bson.D{{Key: "revoked_at", Value: time.Now().UTC()}}}}
	res, err := d.tokensCol().UpdateOne(ctx, filter, update)
	if err != nil {
		return false, fmt.Errorf("revoke token: %w", err)
	}
	return res.MatchedCount > 0, nil
}

func (d *DB) FindTokenByHash(ctx context.Context, hash string) (*models.Token, error) {
	filter := bson.D{
		{Key: "token_hash", Value: hash},
		{Key: "revoked_at", Value: nil},
	}
	var t models.Token
	err := d.tokensCol().FindOne(ctx, filter).Decode(&t)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find token by hash: %w", err)
	}
	return &t, nil
}

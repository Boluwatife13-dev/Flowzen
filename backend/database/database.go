// Package database holds the MongoDB connection.
package database

import (
	"context"
	"log"
	"net"
	"time"

	"flowzen/config"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var DB *mongo.Database
var Ctx = context.Background()

// Connect opens the connection, checks it works and creates the indexes.
func Connect() error {
	useDNSServer(config.Get("DNS_SERVER", ""))
	client, err := mongo.Connect(options.Client().ApplyURI(config.Get("MONGO_URI", "mongodb://localhost:27017")))
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(Ctx, 20*time.Second)
	defer cancel()
	if err := client.Ping(ctx, nil); err != nil {
		return err
	}
	DB = client.Database(config.Get("DB_NAME", "flowzen"))
	Col("users").Indexes().CreateOne(Ctx, mongo.IndexModel{Keys: bson.M{"email": 1}, Options: options.Index().SetUnique(true)})
	return nil
}

// useDNSServer makes the app look up addresses (like your Atlas cluster) through a chosen
// DNS server, e.g. DNS_SERVER=8.8.8.8, instead of the computer's own DNS.
// Leave DNS_SERVER empty to use the computer's normal DNS.
func useDNSServer(server string) {
	if server == "" {
		return
	}
	if _, _, err := net.SplitHostPort(server); err != nil {
		server = net.JoinHostPort(server, "53")
	}
	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			dialer := net.Dialer{Timeout: 5 * time.Second}
			return dialer.DialContext(ctx, network, server)
		},
	}
	log.Println("using DNS server", server)
}

func Col(name string) *mongo.Collection { return DB.Collection(name) }

// Log prints an event and saves it for the System Admin page.
func Log(level, message string) {
	log.Printf("[%s] %s", level, message)
	Col("logs").InsertOne(Ctx, bson.M{"level": level, "message": message, "createdAt": time.Now()})
}
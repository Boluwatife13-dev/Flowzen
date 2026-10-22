package main

import (
	"bufio"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type M = bson.M
type ID = bson.ObjectID

var db *mongo.Database
var cfg = map[string]string{}

func conf(k string) string { return cfg[k] }

func loadConfig() {
	defaults := map[string]string{
		"PORT": "8080", "MONGO_URI": "mongodb://localhost:27017", "DB_NAME": "flowzen",
		"JWT_SECRET": "change-this-secret", "FRONTEND_URL": "http://localhost:8080", "FRONTEND_DIR": "../frontend",
		"SMTP_HOST": "", "SMTP_PORT": "587", "SMTP_USER": "", "SMTP_PASS": "", "SMTP_FROM": "Flowzen <no-reply@flowzen.app>",
		"UPLOAD_DIR": "uploads", "SYSADMIN_EMAIL": "admin@flowzen.app", "SYSADMIN_PASSWORD": "ChangeMe123!",
	}
	if f, err := os.Open(".env"); err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if k, v, ok := strings.Cut(line, "="); ok && !strings.HasPrefix(line, "#") && os.Getenv(strings.TrimSpace(k)) == "" {
				os.Setenv(strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"`))
			}
		}
		f.Close()
	}
	for k, d := range defaults {
		cfg[k] = d
		if v := os.Getenv(k); v != "" {
			cfg[k] = v
		}
	}
	cfg["FRONTEND_URL"] = strings.TrimRight(cfg["FRONTEND_URL"], "/")
}

func main() {
	loadConfig()
	client, err := mongo.Connect(options.Client().ApplyURI(conf("MONGO_URI")).SetServerSelectionTimeout(10 * time.Second))
	if err != nil {
		log.Fatal("mongo connect: ", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := client.Ping(ctx, nil); err != nil {
		log.Fatal("mongo ping: ", err)
	}
	cancel()
	db = client.Database(conf("DB_NAME"))
	os.MkdirAll(filepath.Join(conf("UPLOAD_DIR"), "avatars"), 0755)
	setupDB()
	go reminderLoop()

	mux := http.NewServeMux()
	routes(mux)
	srv := &http.Server{Addr: ":" + conf("PORT"), Handler: cors(mux), ReadHeaderTimeout: 15 * time.Second}
	go func() {
		log.Println("Flowzen API running on :" + conf("PORT"))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	c, cl := context.WithTimeout(context.Background(), 5*time.Second)
	defer cl()
	srv.Shutdown(c)
	client.Disconnect(c)
	log.Println("server stopped")
}

func setupDB() {
	c := context.Background()
	db.Collection("users").Indexes().CreateOne(c, mongo.IndexModel{Keys: bson.D{{Key: "email", Value: 1}}, Options: options.Index().SetUnique(true)})
	if n, _ := db.Collection("users").CountDocuments(c, M{"role": "sysadmin"}); n == 0 {
		hash, _ := hashPassword(conf("SYSADMIN_PASSWORD"))
		_, err := db.Collection("users").InsertOne(c, User{ID: bson.NewObjectID(), Name: "System Admin", Email: strings.ToLower(conf("SYSADMIN_EMAIL")),
			Password: hash, Role: "sysadmin", Active: true, Verified: true, CreatedAt: time.Now()})
		if err == nil {
			log.Println("seeded system administrator:", conf("SYSADMIN_EMAIL"))
		}
	}
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Auth uses a Bearer token (no cookies), so any origin may call the API safely.
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Expose-Headers", "Content-Disposition")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		defer func() {
			if rec := recover(); rec != nil {
				sysLog("error", "panic on "+r.Method+" "+r.URL.Path)
				fail(w, 500, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// ---------- response / request helpers ----------

func send(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
func ok(w http.ResponseWriter, data any)           { send(w, 200, M{"success": true, "data": data}) }
func msg(w http.ResponseWriter, m string)          { send(w, 200, M{"success": true, "message": m}) }
func fail(w http.ResponseWriter, code int, m string) { send(w, code, M{"success": false, "error": m}) }

func body(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(v); err != nil {
		fail(w, 400, "invalid request body")
		return false
	}
	return true
}

func oid(s string) (ID, bool) {
	id, err := bson.ObjectIDFromHex(s)
	return id, err == nil
}

func oids(list []string) ([]ID, bool) {
	out := []ID{}
	seen := map[string]bool{}
	for _, s := range list {
		id, good := oid(s)
		if !good {
			return nil, false
		}
		if !seen[s] {
			seen[s] = true
			out = append(out, id)
		}
	}
	return out, true
}

func pathID(w http.ResponseWriter, r *http.Request, name string) (ID, bool) {
	id, good := oid(r.PathValue(name))
	if !good {
		fail(w, 400, "invalid id")
	}
	return id, good
}

func col(n string) *mongo.Collection { return db.Collection(n) }

func findAll[T any](c context.Context, coll string, filter any, sort bson.D, limit int64) []T {
	o := options.Find()
	if sort != nil {
		o.SetSort(sort)
	}
	if limit > 0 {
		o.SetLimit(limit)
	}
	out := []T{}
	cur, err := col(coll).Find(c, filter, o)
	if err != nil {
		sysLog("error", "db find "+coll+": "+err.Error())
		return out
	}
	if err := cur.All(c, &out); err != nil {
		sysLog("error", "db decode "+coll+": "+err.Error())
	}
	return out
}

func findOne[T any](c context.Context, coll string, filter any) (*T, bool) {
	var v T
	if err := col(coll).FindOne(c, filter).Decode(&v); err != nil {
		return nil, false
	}
	return &v, true
}

func paging(r *http.Request) (int, int) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 500 {
		limit = 10
	}
	return page, limit
}

func paginate[T any](items []T, page, limit int) M {
	total := len(items)
	start, end := (page-1)*limit, page*limit
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}
	return M{"items": items[start:end], "total": total, "page": page, "pages": (total + limit - 1) / limit}
}

func validDate(s string) bool {
	if s == "" {
		return true
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

func today() string { return time.Now().Format("2006-01-02") }

func in(v string, list ...string) bool {
	for _, x := range list {
		if v == x {
			return true
		}
	}
	return false
}

func containsID(list []ID, id ID) bool {
	for _, x := range list {
		if x == id {
			return true
		}
	}
	return false
}

func sysLog(level, message string) {
	log.Printf("[%s] %s", strings.ToUpper(level), message)
	go col("logs").InsertOne(context.Background(), M{"level": level, "message": message, "createdAt": time.Now()})
}

func uniq(ids []ID) []ID {
	out := []ID{}
	for _, id := range ids {
		if !containsID(out, id) {
			out = append(out, id)
		}
	}
	return out
}
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Shamiul-isl/chirpy/internal/database"
	"github.com/google/uuid"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	fileserverHits  atomic.Int32
	DatabaseQueries *database.Queries
	Platform        string
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	// cfg.fileserverHits.Add(1)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) GetMetrics() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(200)
		fmt.Fprintf(w,
			"<html> <body> <h1>Welcome, Chirpy Admin</h1> <p>Chirpy has been visited %d times!</p> </body> </html>", cfg.fileserverHits.Load())
	})
}

func (cfg *apiConfig) ResetMetrics() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cfg.Platform != "dev" {
			log.Fatal("Can't delete users from non local environment")
			w.WriteHeader(403)
			return
		}

		err := cfg.DatabaseQueries.DeleteUsers(r.Context())
		if err != nil {
			log.Fatal(err)
		}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		cfg.fileserverHits.Store(0)
		// w.Write([]byte(fmt.Sprintf("Hits: %d", cfg.fileserverHits.Load())))
	})
}

func cleanBody(str string) string {
	split := strings.Split(str, " ")
	// log.Printf("%q", split)
	for i, s := range split {
		s = strings.ToLower(s)
		if s == "kerfuffle" || s == "sharbert" || s == "fornax" {
			split[i] = "****"
		}
	}

	return strings.Join(split, " ")
}

func main() {
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	platform := os.Getenv("PLATFORM")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal(err)
	}
	dbQueries := database.New(db)

	apiCfg := apiConfig{}
	apiCfg.DatabaseQueries = dbQueries
	apiCfg.Platform = platform
	mux := http.NewServeMux()
	mux.Handle("/app/", apiCfg.middlewareMetricsInc(http.StripPrefix("/app/", http.FileServer(http.Dir(".")))))
	mux.Handle("/assets", http.FileServer(http.Dir("./assets")))
	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		w.Write([]byte("OK"))
	})
	mux.Handle("GET /admin/metrics", apiCfg.GetMetrics())
	mux.Handle("POST /admin/reset", apiCfg.ResetMetrics())
	mux.HandleFunc("POST /api/users", func(w http.ResponseWriter, r *http.Request) {
		type parameter struct {
			Email string `json:"email"`
		}

		type User struct {
			ID        uuid.UUID `json:"id"`
			CreatedAt time.Time `json:"created_at"`
			UpdatedAt time.Time `json:"updated_at"`
			Email     string    `json:"email"`
		}

		decoder := json.NewDecoder(r.Body)
		param := parameter{}
		err := decoder.Decode(&param)
		if err != nil {
			log.Printf("Error decoding parameter: %s", err)
			w.WriteHeader(500)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		user, err := apiCfg.DatabaseQueries.CreateUser(r.Context(), param.Email)
		if err != nil {
			log.Fatal(err)
			w.WriteHeader(500)
			return
		}

		returnuser := User{
			ID:        user.ID,
			CreatedAt: user.CreatedAt,
			UpdatedAt: user.UpdatedAt,
			Email:     user.Email,
		}
		ru, err := json.Marshal(returnuser)
		if err != nil {
			log.Fatal(err)
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(201)
		w.Write(ru)

	})

	mux.HandleFunc("POST /api/chirps", func(w http.ResponseWriter, r *http.Request) {
		type parameter struct {
			Body   string    `json:"body"`
			Userid uuid.UUID `json:"user_id"`
		}

		type Chirp struct {
			ID        uuid.UUID `json:"id"`
			CreatedAt time.Time `json:"created_at"`
			UpdatedAt time.Time `json:"updated_at"`
			Body      string    `json:"body"`
			Userid    uuid.UUID `json:"user_id"`
		}

		decoder := json.NewDecoder(r.Body)
		param := parameter{}
		err := decoder.Decode(&param)
		if err != nil {
			log.Printf("Error decoding parameter: %s", err)
			w.WriteHeader(500)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		if len(param.Body) > 140 {
			log.Fatal("Chirp is too long! Need to be less than 140 characters")
			w.WriteHeader(500)
			return
		}
		param.Body = cleanBody(param.Body)

		chirp, err := apiCfg.DatabaseQueries.CreateChirp(r.Context(), database.CreateChirpParams{
			Body:   param.Body,
			UserID: uuid.NullUUID{UUID: param.Userid, Valid: true},
		})
		if err != nil {
			log.Fatal(err)
			w.WriteHeader(500)
			return
		}

		returnchirp := Chirp{
			ID:        chirp.ID,
			CreatedAt: chirp.CreatedAt,
			UpdatedAt: chirp.UpdatedAt,
			Body:      chirp.Body,
			Userid:    chirp.UserID.UUID,
		}
		ru, err := json.Marshal(returnchirp)
		if err != nil {
			log.Fatal(err)
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(201)
		w.Write(ru)

	})

	mux.HandleFunc("GET /api/chirps", func(w http.ResponseWriter, r *http.Request) {
		type Chirp struct {
			ID        uuid.UUID `json:"id"`
			CreatedAt time.Time `json:"created_at"`
			UpdatedAt time.Time `json:"updated_at"`
			Body      string    `json:"body"`
			Userid    uuid.UUID `json:"user_id"`
		}

		w.Header().Set("Content-Type", "application/json")

		chirps, err := apiCfg.DatabaseQueries.GetAllChirps(r.Context())
		if err != nil {
			log.Fatal(err)
			w.WriteHeader(500)
			return
		}

		allChirps := []Chirp{}

		for _, c := range chirps {
			newchirp := Chirp{
				ID:        c.ID,
				CreatedAt: c.CreatedAt,
				UpdatedAt: c.UpdatedAt,
				Body:      c.Body,
				Userid:    c.UserID.UUID,
			}

			allChirps = append(allChirps, newchirp)
		}

		ru, err := json.Marshal(allChirps)
		if err != nil {
			log.Fatal(err)
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(200)
		w.Write(ru)

	})

	mux.HandleFunc("GET /api/chirps/{chirpID}", func(w http.ResponseWriter, r *http.Request) {
		type Chirp struct {
			ID        uuid.UUID `json:"id"`
			CreatedAt time.Time `json:"created_at"`
			UpdatedAt time.Time `json:"updated_at"`
			Body      string    `json:"body"`
			Userid    uuid.UUID `json:"user_id"`
		}

		getID, err := uuid.Parse(r.PathValue("chirpID"))
		if err != nil {
			log.Fatal(err)
			w.WriteHeader(500)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		chirp, err := apiCfg.DatabaseQueries.GetChirp(r.Context(), getID)
		if err != nil {
			// log.Fatal(err)
			log.Println(err)
			w.WriteHeader(404)
			return
		}

		newchirp := Chirp{
			ID:        chirp.ID,
			CreatedAt: chirp.CreatedAt,
			UpdatedAt: chirp.UpdatedAt,
			Body:      chirp.Body,
			Userid:    chirp.UserID.UUID,
		}

		ru, err := json.Marshal(newchirp)
		if err != nil {
			log.Fatal(err)
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(200)
		w.Write(ru)

	})

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}
	log.Fatal(server.ListenAndServe())
}

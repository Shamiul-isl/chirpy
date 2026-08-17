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

	"github.com/Shamiul-isl/chirpy/internal/database"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	fileserverHits  atomic.Int32
	DatabaseQueries *database.Queries
}

type returnError struct {
	Error string `json:"error"`
}

type returnVal struct {
	Cleaned string `json:"cleaned_body"`
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
		w.Write([]byte(fmt.Sprintf(
			"<html> <body> <h1>Welcome, Chirpy Admin</h1> <p>Chirpy has been visited %d times!</p> </body> </html>", cfg.fileserverHits.Load())))
	})
}

func (cfg *apiConfig) ResetMetrics() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		cfg.fileserverHits.Store(0)
		// w.Write([]byte(fmt.Sprintf("Hits: %d", cfg.fileserverHits.Load())))
	})
}

func respondWithError(w http.ResponseWriter, code int, msg string) {
	returnerr := returnError{Error: msg}
	dat, err := json.Marshal(returnerr)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(500)
		return
	}
	w.WriteHeader(code)
	w.Write(dat)
}

func respondWithJson(w http.ResponseWriter, code int, payload interface{}) {
	dat, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(500)
		return
	}
	w.WriteHeader(code)
	w.Write(dat)
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
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal(err)
	}
	dbQueries := database.New(db)

	apiCfg := apiConfig{}
	apiCfg.DatabaseQueries = dbQueries
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
	mux.HandleFunc("POST /api/validate_chirp", func(w http.ResponseWriter, r *http.Request) {
		type parameter struct {
			Body string `json:"body"`
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
			respondWithError(w, 400, "Chirp is too long")
		} else {
			// log.Printf("%s", param.Body)
			respondWithJson(w, 200, returnVal{Cleaned: cleanBody(param.Body)})
		}
	})
	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}
	log.Fatal(server.ListenAndServe())
}

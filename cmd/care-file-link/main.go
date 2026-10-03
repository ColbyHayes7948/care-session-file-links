package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"

	"github.com/example/care-file-link/internal/appointments"
	"github.com/example/care-file-link/internal/infrai"
)

const bucketName = "care-private-files"

func main() {
	client, err := infrai.New(infrai.DefaultBaseURL, os.Getenv("INFRAI_API_KEY"))
	if err != nil {
		log.Fatal(err)
	}
	if err := client.EnsureBucket(context.Background(), bucketName); err != nil {
		log.Fatal(err)
	}

	service := appointments.NewService(client, bucketName)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /downloads", downloadHandler(service))

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Printf("care file link service listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

type linkIssuer interface {
	IssueLink(context.Context, appointments.DownloadRequest) (appointments.DownloadResult, error)
}

func downloadHandler(service linkIssuer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in appointments.DownloadRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&in); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}

		result, err := service.IssueLink(r.Context(), in)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, appointments.ErrUnauthorized) {
				status = http.StatusForbidden
			} else {
				var remote *infrai.Error
				if errors.As(err, &remote) {
					if remote.Status >= 400 && remote.Status < 500 {
						status = remote.Status
					} else {
						status = http.StatusBadGateway
					}
				}
			}
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write response: %v", err)
	}
}

package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

var (
	entries       []SignalLogEntry
	entriesMu     sync.RWMutex
	lastTimestamp int64
)

// mirror your SignalLogEntry fields & tags
type SignalLogEntry struct {
	ID        string  `firestore:"id" json:"id"`
	Timestamp int64   `firestore:"timestamp" json:"timestamp"`
	Latitude  float64 `firestore:"latitude" json:"latitude"`
	Longitude float64 `firestore:"longitude" json:"longitude"`
	SignalDbm int     `firestore:"signalDbm" json:"signalDbm"`
	Carrier   string  `firestore:"carrier" json:"carrier"`
	Mcc       string  `firestore:"mcc" json:"mcc"`
	Mnc       string  `firestore:"mnc" json:"mnc"`
	CellInfo  string  `firestore:"cellInfo" json:"cellInfo"`
}

// loadAll pulls every doc once at startup into entries[]
func loadAll(ctx context.Context, client *firestore.Client) error {
	q := client.Collection("signal_logs").OrderBy("timestamp", firestore.Asc)
	iter := q.Documents(ctx)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		var e SignalLogEntry
		if err := doc.DataTo(&e); err != nil {
			continue
		}
		entries = append(entries, e)
		lastTimestamp = e.Timestamp
	}
	return nil
}

// saveJSON writes entries[] out to data.json
func saveJSON() error {
	entriesMu.RLock()
	defer entriesMu.RUnlock()
	f, err := os.Create("data.json")
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(entries)
}

func main() {
	ctx := context.Background()

	projectID := "cellsignalmapper-a9da1"

	// use the JSON key pointed to by GOOGLE_APPLICATION_CREDENTIALS
	client, err := firestore.NewClient(ctx, projectID, option.WithCredentialsFile(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")))
	if err != nil {
		log.Fatalf("firestore.NewClient: %v", err)
	}
	defer client.Close()

	// load entire collection once at startup
	if err := loadAll(ctx, client); err != nil {
		log.Fatalf("loadAll failed: %v", err)
	}
	// write initial data.json
	if err := saveJSON(); err != nil {
		log.Fatalf("saveJSON failed: %v", err)
	}

	go func() {
		for {
			time.Sleep(10 * time.Second)
			// query only docs newer than lastTimestamp
			iter := client.Collection("signal_logs").
				Where("timestamp", ">", lastTimestamp).
				OrderBy("timestamp", firestore.Asc).
				Documents(ctx)

			for {
				doc, err := iter.Next()
				if err == iterator.Done {
					break
				}
				if err != nil {
					log.Printf("fetch new error: %v", err)
					break
				}
				var e SignalLogEntry
				if err := doc.DataTo(&e); err != nil {
					continue
				}

				// ←——– Log the new entry!
				log.Printf("📥  New entry: ID=%s, ts=%d, signal=%ddBm, carrier=%s",
					e.ID, e.Timestamp, e.SignalDbm, e.Carrier,
				)

				// append and bump lastTimestamp under lock
				entriesMu.Lock()
				entries = append(entries, e)
				if e.Timestamp > lastTimestamp {
					lastTimestamp = e.Timestamp
				}
				entriesMu.Unlock()
			}

			// save updated JSON
			if err := saveJSON(); err != nil {
				log.Printf("saveJSON error: %v", err)
			}
		}
	}()

	http.Handle("/", http.FileServer(http.Dir("static")))

	http.HandleFunc("/data", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		entriesMu.RLock()
		defer entriesMu.RUnlock()
		if err := json.NewEncoder(w).Encode(entries); err != nil {
			log.Printf("json.Encode: %v", err)
		}
	})

	// start HTTP server
	addr := ":8082"
	srv := &http.Server{
		Addr:         addr,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	log.Printf("Serving on http://localhost%s …", addr)
	log.Fatal(srv.ListenAndServe())
}

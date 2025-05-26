package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

type SignalLog struct {
	Latitude  float64 `firestore:"latitude"`
	Longitude float64 `firestore:"longitude"`
	SignalDbm int     `firestore:"signalDbm"`
	Timestamp int64   `firestore:"timestamp"`
	Carrier   string  `firestore:"carrier"`
}

type GeoJSONFeature struct {
	Type       string                 `json:"type"`
	Geometry   map[string]interface{} `json:"geometry"`
	Properties map[string]interface{} `json:"properties"`
}

type GeoJSONFeatureCollection struct {
	Type     string           `json:"type"`
	Features []GeoJSONFeature `json:"features"`
}

func roundCoord(coord float64, precision int) float64 {
	factor := math.Pow(10, float64(precision))
	return math.Round(coord*factor) / factor
}

func readLastTimestamp(path string) int64 {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0 // default to 0 if file doesn't exist
	}
	ts, err := strconv.ParseInt(string(data), 10, 64)
	if err != nil {
		return 0
	}
	return ts
}

func writeLastTimestamp(path string, ts int64) {
	os.WriteFile(path, []byte(fmt.Sprintf("%d", ts)), 0644)
}

func main() {
	ctx := context.Background()

	// Set up credentials using the firebase-key.json file
	os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "firebase-key.json")

	client, err := firestore.NewClient(ctx, "cellsignalmapper-a9da1")
	if err != nil {
		log.Fatalf("Failed to create Firestore client: %v", err)
	}
	defer client.Close()

	lastTS := readLastTimestamp("public/last_timestamp.txt")
	iter := client.Collection("signal_logs").Where("timestamp", ">", lastTS).Documents(ctx)

	type key struct {
		Lat float64
		Lon float64
	}

	deduped := make(map[key]SignalLog)
	var maxTS int64 = lastTS

	// Load existing heatmap if present
	if data, err := os.ReadFile("public/heatmap.json"); err == nil {
		var existing GeoJSONFeatureCollection
		if err := json.Unmarshal(data, &existing); err == nil {
			for _, f := range existing.Features {
				coords := f.Geometry["coordinates"].([]interface{})
				lon := coords[0].(float64)
				lat := coords[1].(float64)
				signal := int(f.Properties["signalDbm"].(float64))
				ts := int64(f.Properties["timestamp"].(float64))
				carrier := f.Properties["carrier"].(string)
				k := key{Lat: lat, Lon: lon}
				deduped[k] = SignalLog{Latitude: lat, Longitude: lon, SignalDbm: signal, Timestamp: ts, Carrier: carrier}
				if ts > maxTS {
					maxTS = ts
				}
			}
		}
	}

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Fatalf("Failed to iterate: %v", err)
		}

		var logEntry SignalLog
		if err := doc.DataTo(&logEntry); err != nil {
			continue
		}

		if logEntry.Latitude == 0 || logEntry.Longitude == 0 || logEntry.SignalDbm == 0 {
			continue // Skip invalid
		}

		lat := roundCoord(logEntry.Latitude, 4)
		lon := roundCoord(logEntry.Longitude, 4)
		k := key{Lat: lat, Lon: lon}

		existing, found := deduped[k]
		if !found || logEntry.SignalDbm > existing.SignalDbm {
			deduped[k] = logEntry
		}

		if logEntry.Timestamp > maxTS {
			maxTS = logEntry.Timestamp
		}
	}

	var features []GeoJSONFeature
	for k, v := range deduped {
		features = append(features, GeoJSONFeature{
			Type: "Feature",
			Geometry: map[string]interface{}{
				"type":        "Point",
				"coordinates": []float64{k.Lon, k.Lat},
			},
			Properties: map[string]interface{}{
				"signalDbm": v.SignalDbm,
				"timestamp": v.Timestamp,
				"carrier":   v.Carrier,
			},
		})
	}

	collection := GeoJSONFeatureCollection{
		Type:     "FeatureCollection",
		Features: features,
	}

	// Create public directory if it doesn't exist
	os.MkdirAll("public", 0755)
	file, err := os.Create("public/heatmap.json")
	if err != nil {
		log.Fatalf("Failed to create file: %v", err)
	}
	defer file.Close()

	// Encode the GeoJSON to the file
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(collection); err != nil {
		log.Fatalf("Failed to write GeoJSON: %v", err)
	}

	// Also create web/public directory if it doesn't exist
	os.MkdirAll("web/public", 0755)

	// Marshal the JSON to a byte array for copying to web/public
	jsonData, err := json.MarshalIndent(collection, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal JSON: %v", err)
	}

	// Write the same data to web/public/heatmap.json
	if err := os.WriteFile("web/public/heatmap.json", jsonData, 0644); err != nil {
		log.Fatalf("Failed to write to web/public/heatmap.json: %v", err)
	}

	writeLastTimestamp("public/last_timestamp.txt", maxTS)
	fmt.Println("✅ Incremental heatmap.json generated and copied to web/public/.")
}

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"

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

func main() {
	ctx := context.Background()

	client, err := firestore.NewClient(ctx, "cellsignalmapper-a9da1")
	if err != nil {
		log.Fatalf("Failed to create Firestore client: %v", err)
	}
	defer client.Close()

	iter := client.Collection("signal_logs").Documents(ctx)
	type key struct {
		Lat float64
		Lon float64
	}

	deduped := make(map[key]SignalLog)

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

	os.MkdirAll("public", 0755)
	file, err := os.Create("public/heatmap.json")
	if err != nil {
		log.Fatalf("Failed to create file: %v", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(collection); err != nil {
		log.Fatalf("Failed to write GeoJSON: %v", err)
	}

	fmt.Println("✅ Full heatmap.json generated.")
}

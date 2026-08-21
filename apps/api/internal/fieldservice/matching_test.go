package fieldservice

import (
	"testing"
)

func TestRankProvidersPrefersCompatibleNearbyHighRated(t *testing.T) {
	lekkiLat, lekkiLng := 6.4474, 3.4723
	ikejaLat, ikejaLng := 6.6018, 3.3515
	pool := "plumber"
	ranked := RankProviders(pool, CustomerLocation{Latitude: &lekkiLat, Longitude: &lekkiLng, Area: "Lekki"}, []Candidate{
		{ProviderID: "far", Name: "Ikeja Plumber", PoolIDs: []string{pool}, Availability: AvailabilityAvailable, Active: true, Latitude: &ikejaLat, Longitude: &ikejaLng, Areas: []string{"Ikeja"}, Rating: 5, JobsCompleted: 80},
		{ProviderID: "best", Name: "Lekki Plumber", PoolIDs: []string{pool}, Availability: AvailabilityAvailable, Active: true, Latitude: &lekkiLat, Longitude: &lekkiLng, Areas: []string{"Lekki"}, Rating: 4.8, JobsCompleted: 40},
		{ProviderID: "busy", Name: "Busy Lekki", PoolIDs: []string{pool}, Availability: AvailabilityBusy, Active: true, Latitude: &lekkiLat, Longitude: &lekkiLng, Areas: []string{"Lekki"}, Rating: 4.9, JobsCompleted: 90, ActiveJobs: 3},
		{ProviderID: "wrong", Name: "Painter", PoolIDs: []string{"painter"}, Availability: AvailabilityAvailable, Active: true, Latitude: &lekkiLat, Longitude: &lekkiLng, Areas: []string{"Lekki"}, Rating: 5, JobsCompleted: 100},
		{ProviderID: "offline", Name: "Offline", PoolIDs: []string{pool}, Availability: AvailabilityOffline, Active: true, Latitude: &lekkiLat, Longitude: &lekkiLng, Areas: []string{"Lekki"}, Rating: 5, JobsCompleted: 100},
	}, DefaultWeights(), 25)
	if len(ranked) < 2 {
		t.Fatalf("expected ranked eligible providers, got %#v", ranked)
	}
	if ranked[0].ProviderID != "best" {
		t.Fatalf("expected lekki plumber first, got %+v", ranked[0])
	}
	for _, row := range ranked {
		if row.ProviderID == "wrong" || row.ProviderID == "offline" {
			t.Fatalf("ineligible provider leaked into ranking: %+v", row)
		}
	}
}

func TestConfigurableWeightsChangeRanking(t *testing.T) {
	lat, lng := 6.45, 3.4
	farLat, farLng := 6.60, 3.35
	pool := "ac"
	distanceHeavy := MatchWeights{Service: 0.1, Availability: 0.1, Distance: 0.8, Rating: 0, Experience: 0}
	ranked := RankProviders(pool, CustomerLocation{Latitude: &lat, Longitude: &lng}, []Candidate{
		{ProviderID: "near-ok", Name: "Near", PoolIDs: []string{pool}, Availability: AvailabilityAvailable, Active: true, Latitude: &lat, Longitude: &lng, Rating: 3.5, JobsCompleted: 2},
		{ProviderID: "far-star", Name: "Far star", PoolIDs: []string{pool}, Availability: AvailabilityAvailable, Active: true, Latitude: &farLat, Longitude: &farLng, Rating: 5, JobsCompleted: 90},
	}, distanceHeavy, 30)
	if ranked[0].ProviderID != "near-ok" {
		t.Fatalf("distance-heavy weights should prefer nearer provider, got %+v", ranked)
	}
}

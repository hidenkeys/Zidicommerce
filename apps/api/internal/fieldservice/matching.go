package fieldservice

import (
	"encoding/json"
	"math"
	"strings"
)

type MatchWeights struct {
	Service      float64
	Availability float64
	Distance     float64
	Rating       float64
	Experience   float64
}

func DefaultWeights() MatchWeights {
	return MatchWeights{Service: 0.40, Availability: 0.20, Distance: 0.20, Rating: 0.15, Experience: 0.05}
}

func (w MatchWeights) normalized() MatchWeights {
	if w.Service == 0 && w.Availability == 0 && w.Distance == 0 && w.Rating == 0 && w.Experience == 0 {
		return DefaultWeights()
	}
	sum := w.Service + w.Availability + w.Distance + w.Rating + w.Experience
	if sum <= 0 {
		return DefaultWeights()
	}
	return MatchWeights{
		Service:      w.Service / sum,
		Availability: w.Availability / sum,
		Distance:     w.Distance / sum,
		Rating:       w.Rating / sum,
		Experience:   w.Experience / sum,
	}
}

type Candidate struct {
	ProviderID    string
	Name          string
	PoolIDs       []string
	Availability  string
	Active        bool
	Latitude      *float64
	Longitude     *float64
	Areas         []string
	Rating        float64
	JobsCompleted int
	ActiveJobs    int
}

type CustomerLocation struct {
	Latitude  *float64
	Longitude *float64
	Area      string
}

type ScoreBreakdown struct {
	ProviderID      string  `json:"provider_id"`
	Name            string  `json:"name"`
	Total           float64 `json:"total"`
	Service         float64 `json:"service"`
	Availability    float64 `json:"availability"`
	Distance        float64 `json:"distance"`
	Rating          float64 `json:"rating"`
	Experience      float64 `json:"experience"`
	WorkloadPenalty float64 `json:"workload_penalty"`
	DistanceKM      float64 `json:"distance_km"`
	Eligible        bool    `json:"eligible"`
	Reason          string  `json:"reason,omitempty"`
}

func RankProviders(poolID string, location CustomerLocation, candidates []Candidate, weights MatchWeights, maxDistanceKM float64) []ScoreBreakdown {
	weights = weights.normalized()
	if maxDistanceKM <= 0 {
		maxDistanceKM = 25
	}
	ranked := make([]ScoreBreakdown, 0, len(candidates))
	for _, candidate := range candidates {
		score := scoreCandidate(poolID, location, candidate, weights, maxDistanceKM)
		if score.Eligible {
			ranked = append(ranked, score)
		}
	}
	for i := 0; i < len(ranked); i++ {
		for j := i + 1; j < len(ranked); j++ {
			if ranked[j].Total > ranked[i].Total {
				ranked[i], ranked[j] = ranked[j], ranked[i]
			}
		}
	}
	return ranked
}

func scoreCandidate(poolID string, location CustomerLocation, candidate Candidate, weights MatchWeights, maxDistanceKM float64) ScoreBreakdown {
	out := ScoreBreakdown{ProviderID: candidate.ProviderID, Name: candidate.Name}
	if !candidate.Active {
		out.Reason = "inactive"
		return out
	}
	if !hasPool(candidate.PoolIDs, poolID) {
		out.Reason = "service_mismatch"
		return out
	}
	service := 1.0
	availability := availabilityScore(candidate.Availability)
	if availability <= 0 {
		out.Reason = "unavailable"
		return out
	}
	distanceKM, distanceScore := distanceComponent(location, candidate, maxDistanceKM)
	if location.Latitude != nil && location.Longitude != nil && candidate.Latitude != nil && candidate.Longitude != nil && distanceKM > maxDistanceKM && !areaCompatible(location.Area, candidate.Areas) {
		out.Reason = "too_far"
		return out
	}
	if location.Latitude == nil && strings.TrimSpace(location.Area) != "" && !areaCompatible(location.Area, candidate.Areas) && distanceScore == 0 {
		distanceScore = 0.4
	}
	rating := candidate.Rating / 5.0
	if rating < 0 {
		rating = 0
	}
	if rating > 1 {
		rating = 1
	}
	experience := float64(candidate.JobsCompleted) / 50.0
	if experience > 1 {
		experience = 1
	}
	workload := 1.0 / (1.0 + float64(candidate.ActiveJobs))
	availabilityWeighted := availability * workload
	total := 100 * (weights.Service*service +
		weights.Availability*availabilityWeighted +
		weights.Distance*distanceScore +
		weights.Rating*rating +
		weights.Experience*experience)
	out.Eligible = true
	out.Total = math.Round(total*10) / 10
	out.Service = math.Round(weights.Service*1000) / 10
	out.Availability = math.Round(weights.Availability*availabilityWeighted*1000) / 10
	out.Distance = math.Round(weights.Distance*distanceScore*1000) / 10
	out.Rating = math.Round(weights.Rating*rating*1000) / 10
	out.Experience = math.Round(weights.Experience*experience*1000) / 10
	out.WorkloadPenalty = math.Round((1-workload)*weights.Availability*1000) / 10
	out.DistanceKM = math.Round(distanceKM*10) / 10
	return out
}

func availabilityScore(value string) float64 {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case AvailabilityAvailable:
		return 1
	case AvailabilityBusy:
		return 0.25
	default:
		return 0
	}
}

func hasPool(poolIDs []string, poolID string) bool {
	for _, id := range poolIDs {
		if id == poolID {
			return true
		}
	}
	return false
}

func areaCompatible(customerArea string, providerAreas []string) bool {
	needle := strings.ToLower(strings.TrimSpace(customerArea))
	if needle == "" {
		return true
	}
	for _, area := range providerAreas {
		if strings.Contains(strings.ToLower(area), needle) || strings.Contains(needle, strings.ToLower(strings.TrimSpace(area))) {
			return true
		}
	}
	return false
}

func distanceComponent(location CustomerLocation, candidate Candidate, maxDistanceKM float64) (float64, float64) {
	if location.Latitude == nil || location.Longitude == nil || candidate.Latitude == nil || candidate.Longitude == nil {
		if areaCompatible(location.Area, append(candidate.Areas, candidate.Name)) {
			return 0, 0.7
		}
		return 0, 0.4
	}
	km := haversineKM(*location.Latitude, *location.Longitude, *candidate.Latitude, *candidate.Longitude)
	score := 1 - (km / maxDistanceKM)
	if score < 0 {
		score = 0
	}
	return km, score
}

func haversineKM(lat1, lon1, lat2, lon2 float64) float64 {
	const earth = 6371.0
	toRad := func(d float64) float64 { return d * math.Pi / 180 }
	dLat := toRad(lat2 - lat1)
	dLon := toRad(lon2 - lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * earth * math.Asin(math.Sqrt(a))
}

func encodeBreakdown(score ScoreBreakdown) string {
	raw, err := json.Marshal(score)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func GeocodeLagos(text string) (float64, float64, bool) {
	value := strings.ToLower(text)
	points := map[string][2]float64{
		"lekki":           {6.4474, 3.4723},
		"ajah":            {6.4698, 3.5646},
		"victoria island": {6.4281, 3.4219},
		"vi":              {6.4281, 3.4219},
		"ikoyi":           {6.4541, 3.4358},
		"yaba":            {6.5095, 3.3711},
		"surulere":        {6.5000, 3.3500},
		"ikeja":           {6.6018, 3.3515},
		"maryland":        {6.5764, 3.3682},
		"gbagada":         {6.5447, 3.3892},
		"magodo":          {6.6183, 3.3823},
		"mainland":        {6.5080, 3.3710},
		"island":          {6.4549, 3.3947},
	}
	for name, point := range points {
		if strings.Contains(value, name) {
			return point[0], point[1], true
		}
	}
	return 0, 0, false
}

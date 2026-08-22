package fieldservice

func defaultDemoProfile() SeedProfile {
	return SeedProfile{
		OrganizationName: DemoCompanyName,
		OrganizationSlug: DemoOrgSlug,
		Owner:            SeedOwnerProfile{Email: DemoOwnerEmail, FirstName: "Ada", LastName: "Okoye"},
		WelcomeMessage:   "Hi 👋 Welcome to Lagos Home Services.\nWe help you find trusted professionals for home services across Lagos.\n\nWhat service do you need today?",
		BookingFeeMinor:  500000, AcceptanceWindow: 120, MaxDistanceKM: 25,
		Weights: SeedMatchingWeights{Service: 0.40, Availability: 0.20, Distance: 0.20, Rating: 0.15, Experience: 0.05},
		Pools:   []string{"Plumber", "Electrician", "Carpenter", "Painter", "AC Technician", "Appliance Repair", "General Handyman", "Other"},
		Providers: []SeedProviderProfile{
			{Code: "john-lekki", Name: "John Adeyemi", Email: "john-lekki@providers.zidicommerce.local", Phone: "+2348000000000", Area: "Lekki", Latitude: 6.4474, Longitude: 3.4723, Pools: []string{"Plumber", "AC Technician"}, Rating: 4.8, RatingCount: 42, JobsCompleted: 42, Availability: AvailabilityAvailable},
			{Code: "michael-lekki", Name: "Michael Okonkwo", Email: "michael-lekki@providers.zidicommerce.local", Phone: "+2348000000000", Area: "Lekki", Latitude: 6.4490, Longitude: 3.4800, Pools: []string{"Electrician"}, Rating: 4.6, RatingCount: 28, JobsCompleted: 28, Availability: AvailabilityAvailable},
			{Code: "samuel-ajah", Name: "Samuel Bello", Email: "samuel-ajah@providers.zidicommerce.local", Phone: "+2348000000000", Area: "Ajah", Latitude: 6.4698, Longitude: 3.5646, Pools: []string{"Plumber"}, Rating: 4.4, RatingCount: 19, JobsCompleted: 19, Availability: AvailabilityAvailable},
			{Code: "david-vi", Name: "David Eze", Email: "david-vi@providers.zidicommerce.local", Phone: "+2348000000000", Area: "Victoria Island", Latitude: 6.4281, Longitude: 3.4219, Pools: []string{"Electrician", "Appliance Repair"}, Rating: 4.9, RatingCount: 61, JobsCompleted: 61, Availability: AvailabilityAvailable},
			{Code: "chidi-ikoyi", Name: "Chidi Nwosu", Email: "chidi-ikoyi@providers.zidicommerce.local", Phone: "+2348000000000", Area: "Ikoyi", Latitude: 6.4541, Longitude: 3.4358, Pools: []string{"Painter"}, Rating: 4.2, RatingCount: 15, JobsCompleted: 15, Availability: AvailabilityAvailable},
			{Code: "tunde-yaba", Name: "Tunde Balogun", Email: "tunde-yaba@providers.zidicommerce.local", Phone: "+2348000000000", Area: "Yaba", Latitude: 6.5095, Longitude: 3.3711, Pools: []string{"Carpenter", "General Handyman"}, Rating: 4.7, RatingCount: 33, JobsCompleted: 33, Availability: AvailabilityBusy},
			{Code: "amina-surulere", Name: "Amina Lawal", Email: "amina-surulere@providers.zidicommerce.local", Phone: "+2348000000000", Area: "Surulere", Latitude: 6.5000, Longitude: 3.3500, Pools: []string{"Appliance Repair"}, Rating: 4.5, RatingCount: 24, JobsCompleted: 24, Availability: AvailabilityAvailable},
			{Code: "ibrahim-ikeja", Name: "Ibrahim Musa", Email: "ibrahim-ikeja@providers.zidicommerce.local", Phone: "+2348000000000", Area: "Ikeja", Latitude: 6.6018, Longitude: 3.3515, Pools: []string{"Plumber", "General Handyman"}, Rating: 4.3, RatingCount: 18, JobsCompleted: 18, Availability: AvailabilityAvailable},
			{Code: "grace-maryland", Name: "Grace Umeh", Email: "grace-maryland@providers.zidicommerce.local", Phone: "+2348000000000", Area: "Maryland", Latitude: 6.5764, Longitude: 3.3682, Pools: []string{"AC Technician"}, Rating: 4.8, RatingCount: 47, JobsCompleted: 47, Availability: AvailabilityAvailable},
			{Code: "peter-gbagada", Name: "Peter Obioma", Email: "peter-gbagada@providers.zidicommerce.local", Phone: "+2348000000000", Area: "Gbagada", Latitude: 6.5447, Longitude: 3.3892, Pools: []string{"Electrician"}, Rating: 3.9, RatingCount: 9, JobsCompleted: 9, Availability: AvailabilityOffline},
			{Code: "nkechi-magodo", Name: "Nkechi Okafor", Email: "nkechi-magodo@providers.zidicommerce.local", Phone: "+2348000000000", Area: "Magodo", Latitude: 6.6183, Longitude: 3.3823, Pools: []string{"Painter", "Carpenter"}, Rating: 4.6, RatingCount: 22, JobsCompleted: 22, Availability: AvailabilityAvailable},
			{Code: "femi-mainland", Name: "Femi Adebayo", Email: "femi-mainland@providers.zidicommerce.local", Phone: "+2348000000000", Area: "Lagos Mainland", Latitude: 6.5080, Longitude: 3.3710, Pools: []string{"Plumber"}, Rating: 4.1, RatingCount: 12, JobsCompleted: 12, Availability: AvailabilityAvailable},
			{Code: "halima-island", Name: "Halima Sule", Email: "halima-island@providers.zidicommerce.local", Phone: "+2348000000000", Area: "Lagos Island", Latitude: 6.4549, Longitude: 3.3947, Pools: []string{"AC Technician", "Electrician"}, Rating: 4.7, RatingCount: 38, JobsCompleted: 38, Availability: AvailabilityAvailable},
			{Code: "kunle-lekki", Name: "Kunle Ajayi", Email: "kunle-lekki@providers.zidicommerce.local", Phone: "+2348000000000", Area: "Lekki", Latitude: 6.4410, Longitude: 3.4680, Pools: []string{"Carpenter"}, Rating: 4.0, RatingCount: 8, JobsCompleted: 8, Availability: AvailabilityAvailable},
			{Code: "blessing-ajah", Name: "Blessing Etuk", Email: "blessing-ajah@providers.zidicommerce.local", Phone: "+2348000000000", Area: "Ajah", Latitude: 6.4720, Longitude: 3.5710, Pools: []string{"Appliance Repair", "Other"}, Rating: 4.4, RatingCount: 16, JobsCompleted: 16, Availability: AvailabilityAvailable},
			{Code: "emeka-ikeja", Name: "Emeka Chukwu", Email: "emeka-ikeja@providers.zidicommerce.local", Phone: "+2348000000000", Area: "Ikeja", Latitude: 6.6050, Longitude: 3.3480, Pools: []string{"Electrician", "AC Technician"}, Rating: 4.9, RatingCount: 72, JobsCompleted: 72, Availability: AvailabilityBusy},
			{Code: "zainab-yaba", Name: "Zainab Abdullahi", Email: "zainab-yaba@providers.zidicommerce.local", Phone: "+2348000000000", Area: "Yaba", Latitude: 6.5120, Longitude: 3.3770, Pools: []string{"Painter"}, Rating: 4.3, RatingCount: 14, JobsCompleted: 14, Availability: AvailabilityAvailable},
			{Code: "segun-surulere", Name: "Segun Adeleke", Email: "segun-surulere@providers.zidicommerce.local", Phone: "+2348000000000", Area: "Surulere", Latitude: 6.4960, Longitude: 3.3550, Pools: []string{"General Handyman", "Plumber"}, Rating: 4.5, RatingCount: 29, JobsCompleted: 29, Availability: AvailabilityAvailable},
			{Code: "rita-gbagada", Name: "Rita Mensah", Email: "rita-gbagada@providers.zidicommerce.local", Phone: "+2348000000000", Area: "Gbagada", Latitude: 6.5480, Longitude: 3.3930, Pools: []string{"Other", "Appliance Repair"}, Rating: 4.2, RatingCount: 11, JobsCompleted: 11, Availability: AvailabilityAvailable},
			{Code: "daniel-ikoyi", Name: "Daniel Wright", Email: "daniel-ikoyi@providers.zidicommerce.local", Phone: "+2348000000000", Area: "Ikoyi", Latitude: 6.4500, Longitude: 3.4310, Pools: []string{"Plumber", "Electrician"}, Rating: 4.8, RatingCount: 54, JobsCompleted: 54, Availability: AvailabilityAvailable},
		},
		Jobs: demoJobs(),
	}
}

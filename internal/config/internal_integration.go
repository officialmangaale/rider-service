package config

import "strings"

// InternalIntegrationIssues lists what is missing for rider-service to talk to
// restaurant-service. Without these, every callback (assignment, pickup,
// delivered, location) fails and the rider sees "Could not reach the
// restaurant system" at Picked up.
//
// It reports names only, never values. The service still starts: dispatch and
// offers do not need the callbacks, so this is a loud warning, not a refusal.
func (c *Config) InternalIntegrationIssues() []string {
	var issues []string
	if strings.TrimSpace(c.RestaurantServiceBaseURL) == "" {
		issues = append(issues, "RESTAURANT_SERVICE_INTERNAL_BASE_URL is empty")
	}
	if strings.TrimSpace(c.InternalServiceToken) == "" {
		issues = append(issues, "INTERNAL_SERVICE_TOKEN is empty")
	}
	return issues
}
